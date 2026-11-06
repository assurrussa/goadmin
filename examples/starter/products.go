package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type productData struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Stock       int64  `json:"stock"`
	Active      bool   `json:"active"`
	Available   string `json:"available"`
	Category    string `json:"category"`
	Status      string `json:"status"`
}

// Status is intentionally absent from writable DTOs: only Publish changes it.
type productInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Stock       int64  `json:"stock"`
	Active      bool   `json:"active"`
	Available   string `json:"available"`
	Category    string `json:"category"`
}

func (p productData) input() productInput {
	return productInput{Name: p.Name, Description: p.Description, Stock: p.Stock, Active: p.Active, Available: p.Available, Category: p.Category}
}

type productRepo struct{ db *sql.DB }

const productColumns = "id::text, revision::text, name, description, stock, active, available, category, status"

func productScan(scanner interface{ Scan(...any) error }) (crud.Record[productData], error) {
	var r crud.Record[productData]
	err := scanner.Scan(&r.ID, &r.Revision, &r.Data.Name, &r.Data.Description, &r.Data.Stock, &r.Data.Active, &r.Data.Available, &r.Data.Category, &r.Data.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return r, crud.ErrNotFound
	}
	return r, err
}

func (r productRepo) get(ctx context.Context, actor host.Actor, id string) (crud.Record[productData], error) {
	return productScan(r.db.QueryRowContext(ctx, "SELECT "+productColumns+" FROM starter_products WHERE owner_admin_id=$1 AND id::text=$2", actor.AdminID, id))
}

func (r productRepo) list(ctx context.Context, actor host.Actor, f datagrid.Filtered) ([]crud.Record[productData], int, error) {
	where, args := "owner_admin_id=$1", []any{actor.AdminID}
	add := func(column string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(" AND %s=$%d", column, len(args))
	}
	if search := f.GetSearch(); search != "" {
		args = append(args, "%"+search+"%")
		where += fmt.Sprintf(" AND name ILIKE $%d", len(args))
	}
	for _, key := range []string{"category", "status", "active", "available", "stock", "name"} {
		if v, ok := f.GetFields()[key]; ok {
			if date, yes := v.(time.Time); yes {
				v = date.Format("2006-01-02")
			}
			add(key, v)
		}
	}
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM starter_products WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sort := map[string]string{"id": "id", "name": "name", "stock": "stock", "active": "active", "available": "available", "status": "status", "category": "category"}[f.GetSortBy()]
	if sort == "" {
		sort = "id"
	}
	order := "ASC"
	if f.GetSortOrder() == "desc" {
		order = "DESC"
	}
	args = append(args, f.GetLimit(), f.GetOffset())
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM starter_products WHERE %s ORDER BY %s %s,id LIMIT $%d OFFSET $%d", productColumns, where, sort, order, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	records := []crud.Record[productData]{}
	for rows.Next() {
		record, e := productScan(rows)
		if e != nil {
			return nil, 0, e
		}
		records = append(records, record)
	}
	return records, total, rows.Err()
}

func validateProduct(p productInput) error {
	errs := crud.FieldErrors{}
	if strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 200 {
		errs["name"] = []string{"Name must contain 1–200 characters"}
	}
	if p.Stock < 0 || p.Stock > 9007199254740991 {
		errs["stock"] = []string{"Stock must be a nonnegative safe integer"}
	}
	if p.Available != "" {
		if _, err := time.Parse("2006-01-02", p.Available); err != nil {
			errs["available"] = []string{"Use YYYY-MM-DD"}
		}
	}
	if p.Category != "" && p.Category != "standard" && p.Category != "premium" {
		errs["category"] = []string{"Choose a category"}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func productAudit(ctx context.Context, tx *sql.Tx, actor host.Actor, id, action string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO starter_product_events(owner_admin_id,product_id,action) VALUES($1,$2,$3)", actor.AdminID, id, action)
	return err
}

func (r productRepo) create(ctx context.Context, a host.Actor, p productInput) (crud.Record[productData], error) {
	if err := validateProduct(p); err != nil {
		return crud.Record[productData]{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return crud.Record[productData]{}, err
	}
	defer tx.Rollback()
	record, err := productScan(tx.QueryRowContext(ctx, "INSERT INTO starter_products(owner_admin_id,name,description,stock,active,available,category,status) VALUES($1,$2,$3,$4,$5,$6,$7,'draft') RETURNING "+productColumns, a.AdminID, p.Name, p.Description, p.Stock, p.Active, p.Available, p.Category))
	if err == nil {
		err = productAudit(ctx, tx, a, record.ID, "created")
	}
	if err != nil {
		return record, err
	}
	if err := tx.Commit(); err != nil {
		return record, fmt.Errorf("%w: %v", crud.ErrUnknown, err)
	}
	return record, nil
}

func (r productRepo) update(ctx context.Context, a host.Actor, id, revision string, p productInput) (crud.Record[productData], error) {
	if err := validateProduct(p); err != nil {
		return crud.Record[productData]{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return crud.Record[productData]{}, err
	}
	defer tx.Rollback()
	record, err := productScan(tx.QueryRowContext(ctx, "UPDATE starter_products SET name=$4,description=$5,stock=$6,active=$7,available=$8,category=$9,revision=revision+1 WHERE owner_admin_id=$1 AND id::text=$2 AND revision::text=$3 RETURNING "+productColumns, a.AdminID, id, revision, p.Name, p.Description, p.Stock, p.Active, p.Available, p.Category))
	if errors.Is(err, crud.ErrNotFound) {
		err = crud.ErrConflict
	}
	if err == nil {
		err = productAudit(ctx, tx, a, id, "updated")
	}
	if err != nil {
		return record, err
	}
	if err := tx.Commit(); err != nil {
		return record, fmt.Errorf("%w: %v", crud.ErrUnknown, err)
	}
	return record, nil
}

func (r productRepo) command(ctx context.Context, a host.Actor, id, revision, action string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := "DELETE FROM starter_products WHERE owner_admin_id=$1 AND id::text=$2 AND revision::text=$3"
	if action == "publish" {
		query = "UPDATE starter_products SET status='published',revision=revision+1 WHERE owner_admin_id=$1 AND id::text=$2 AND revision::text=$3 AND status='draft'"
	}
	result, err := tx.ExecContext(ctx, query, a.AdminID, id, revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return crud.ErrConflict
	}
	if err = productAudit(ctx, tx, a, id, action); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", crud.ErrUnknown, err)
	}
	return nil
}

func productsModule(db *sql.DB) (host.Module, error) {
	r := productRepo{db}
	permission := func(action host.PermissionAction) host.PermissionKey {
		return host.NewPermissionKey("products", action)
	}
	fields := []crud.Field{
		{Key: "name", Label: "Name", Type: "text", Read: true, Create: true, Update: true, Required: true},
		{Key: "description", Label: "Description", Type: "textarea", Read: true, Create: true, Update: true},
		{Key: "stock", Label: "Stock", Type: "number", Read: true, Create: true, Update: true},
		{Key: "active", Label: "Active", Type: "boolean", Read: true, Create: true, Update: true},
		{Key: "available", Label: "Available", Type: "date", Read: true, Create: true, Update: true},
		{
			Key: "category", Label: "Category", Type: "select", Read: true, Create: true, Update: true,
			Options: []crud.Option{{Value: "standard", Label: "Standard"}, {Value: "premium", Label: "Premium"}},
		},
		{
			Key: "status", Label: "Status", Type: "select", Read: true,
			Options: []crud.Option{{Value: "draft", Label: "Draft"}, {Value: "published", Label: "Published"}},
		},
	}
	return crud.New(crud.Definition[productData, productInput, productInput]{
		Key: "products", BasePath: "/products", Title: "Products", Fields: fields, PageSize: 20, DefaultSort: "name", DefaultOrder: "asc",
		Columns: []datagrid.Column{
			{Key: "name", Label: "Name", Type: "text", Sortable: true},
			{Key: "stock", Label: "Stock", Type: "number", Sortable: true, Filterable: true},
			{Key: "active", Label: "Active", Type: "boolean", Filterable: true},
			{Key: "available", Label: "Available", Type: "date", Sortable: true, Filterable: true},
			{Key: "category", Label: "Category", Type: "select", Filterable: true, FilterOptions: []map[string]string{{"value": "standard", "label": "Standard"}, {"value": "premium", "label": "Premium"}}},
			{Key: "status", Label: "Status", Type: "select", Filterable: true, FilterOptions: []map[string]string{{"value": "draft", "label": "Draft"}, {"value": "published", "label": "Published"}}},
		},
		Permissions: crud.Permissions{Read: permission("read"), Create: permission("create"), Update: permission("update"), Delete: permission("delete")},
		Menu:        host.Menu{Sections: []host.Section{{Key: "products", Title: "Products", Items: []host.Item{{Name: "Products", Href: "/products", Icon: "users", PermissionKey: permission("read")}}}}},
		List:        r.list, Get: r.get, LoadSelected: func(ctx context.Context, a host.Actor, ids []string) ([]crud.Record[productData], error) {
			records := []crud.Record[productData]{}
			for _, id := range ids {
				record, err := r.get(ctx, a, id)
				if errors.Is(err, crud.ErrNotFound) {
					continue
				}
				if err != nil {
					return nil, err
				}
				records = append(records, record)
			}
			return records, nil
		},
		Create: r.create, Update: r.update,
		Delete: func(ctx context.Context, a host.Actor, id, revision string) error {
			return r.command(ctx, a, id, revision, "deleted")
		},
		Authorize: func(_ context.Context, _ host.Actor, op crud.Operation, record crud.Record[productData]) error {
			if op == "publish" && record.Data.Status != "draft" {
				return crud.ErrDenied
			}
			return nil
		},
		Actions: []crud.Action[productData]{{Key: "publish", Label: "Publish", Confirmation: "Publish this product?", Permission: permission("publish"), Run: func(ctx context.Context, a host.Actor, id, revision string) error {
			return r.command(ctx, a, id, revision, "publish")
		}}},
	})
}

func migrateProducts(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(76110061)"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS starter_product_migrations(version bigint PRIMARY KEY)"); err != nil {
		return err
	}
	changes := []struct {
		version    int
		statements []string
	}{
		{1, []string{
			"CREATE TABLE IF NOT EXISTS starter_products(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_admin_id bigint NOT NULL REFERENCES administrations(id) ON DELETE CASCADE,revision bigint NOT NULL DEFAULT 1,name text NOT NULL,description text NOT NULL,stock bigint NOT NULL,active boolean NOT NULL,available text NOT NULL,status text NOT NULL)",
			"CREATE TABLE IF NOT EXISTS starter_product_events(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_admin_id bigint NOT NULL,product_id text NOT NULL,action text NOT NULL,created_at timestamptz NOT NULL DEFAULT NOW())",
		}},
		{2, []string{"ALTER TABLE starter_products ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT ''"}},
	}
	for _, change := range changes {
		var installed bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM starter_product_migrations WHERE version=$1)", change.version).Scan(&installed); err != nil {
			return err
		}
		if installed {
			continue
		}
		for _, statement := range change.statements {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO starter_product_migrations(version) VALUES($1)", change.version); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", crud.ErrUnknown, err)
	}
	return nil
}
