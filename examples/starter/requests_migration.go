package main

import (
	"context"
	"database/sql"
)

func migrateRequests(ctx context.Context, dsn string) error {
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
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(76110062)"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS starter_request_migrations(version bigint PRIMARY KEY)"); err != nil {
		return err
	}
	var applied bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM starter_request_migrations WHERE version=1)").Scan(&applied); err != nil {
		return err
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE starter_requests (
				id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
				revision bigint NOT NULL DEFAULT 1,
				subject text NOT NULL,
				status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected'))
			);
			CREATE TABLE starter_request_decisions (
				request_id bigint PRIMARY KEY REFERENCES starter_requests(id),
				status text NOT NULL CHECK (status IN ('approved','rejected')),
				admin_id bigint NOT NULL CHECK (admin_id>0),
				decided_at timestamptz NOT NULL DEFAULT now(),
				reason text,
				CHECK ((status='approved' AND reason IS NULL) OR (status='rejected' AND reason IS NOT NULL AND reason<>''))
			);
			INSERT INTO starter_requests(subject) VALUES
				('Доступ к архиву'), ('Доступ к ведомости'), ('Доступ к договору'),
				('Доступ к журналу'), ('Доступ к каталогу'), ('Доступ к отчёту');
			INSERT INTO starter_request_migrations(version) VALUES (1);`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
