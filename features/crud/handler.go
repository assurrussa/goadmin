package crud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strings"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/gridworkflow"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type handler[R, C, U any] struct {
	app *host.App
	d   Definition[R, C, U]
}
type projected = map[string]any

//nolint:gocognit // Explicit route and validation branches keep authorization order visible.
func (h *handler[R, C, U]) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	p := h.d.Permissions
	group := router.Group(h.d.BasePath, h.app.Guard(p.Read.Domain, p.Read.Action))
	cfg := datagrid.FilterConfig[projected]{
		RoutePath:         h.d.BasePath,
		Entity:            h.d.Key,
		Title:             h.d.Title,
		Columns:           h.d.Columns,
		DefaultSort:       h.d.DefaultSort,
		DefaultOrder:      h.d.DefaultOrder,
		PageSize:          h.d.PageSize,
		Selectable:        true,
		Exportable:        true,
		Refreshable:       true,
		IDKey:             "id",
		Creatable:         true,
		CanCreate:         h.canCreate,
		APIErrorComponent: h.apiFailure,
	}
	cfg.Actions = []datagrid.Action[projected]{
		{Key: "view", Label: "Просмотр"},
		{
			Key:   "edit",
			Label: "Редактировать",
			CanView: func(ctx context.Context,
				row projected,
			) bool {
				return h.rowCan(ctx,
					row,
					Update,
					p.Update)
			},
		},

		{
			Key:   string(Delete),
			Label: "Удалить",
			CanView: func(ctx context.Context,
				row projected,
			) bool {
				return h.rowCan(ctx,
					row,
					Delete,
					p.Delete)
			},
		},
	}
	for _, action := range h.d.Actions {
		cfg.Actions = append(cfg.Actions,
			datagrid.Action[projected]{
				Key:   action.Key,
				Label: action.Label,
				CanView: func(ctx context.Context,
					row projected,
				) bool {
					return h.rowCan(ctx, row, Operation(action.Key), action.Permission)
				},
			})
	}
	cfg.Repository = repository[projected](func(ctx context.Context, f datagrid.Filtered) ([]projected, int, error) {
		actor, ok := h.app.CurrentActor(ctx)
		if !ok {
			return nil, 0, ErrDenied
		}
		records, total, err := h.d.List(ctx, actor, f)
		if err != nil {
			return nil, 0, err
		}
		if total < 0 || len(records) > f.GetLimit() {
			return nil, 0, errors.New("crud: invalid page result")
		}
		rows, err := h.projectMany(ctx, actor, records)
		return rows, total, err
	})
	cfg.PageComponent = func(c fiber.Ctx, data datagrid.Response[projected]) error {
		props := h.props(c)
		props["data"] = data.ToAPIResponse()
		return h.app.Render(c, "crud/Index", props)
	}
	cfg.ErrorComponent = func(c fiber.Ctx, _ datagrid.ErrorData, err error) error { return h.failure(c, err, nil) }
	grid := datagrid.NewHandler(cfg, h.app.Logger())
	_ = gridworkflow.Register(group,
		grid,
		gridworkflow.Options[projected]{
			Key:   "crud." + h.d.Key,
			Store: gridworkflow.NewPostgresStore(h.app.Unwrap().GridDB()),
			CurrentAdmin: func(c fiber.Ctx) (int64,
				error,
			) {
				a, ok := h.app.CurrentActor(c)
				if !ok {
					return 0, ErrDenied
				}
				return a.AdminID, nil
			},
			SelectionID: func(r projected) string {
				id,
					_ := r["id"].(string)
				return id
			},
			LoadSelected: func(ctx context.Context,
				ids []string) ([]projected,
				error,
			) {
				a, ok := h.app.CurrentActor(ctx)
				if !ok {
					return nil, ErrDenied
				}
				rows, err := h.d.LoadSelected(ctx, a, ids)
				if err != nil {
					return nil, err
				}
				wanted := map[string]bool{}
				for _, id := range ids {
					wanted[id] = true
				}
				for _, r := range rows {
					if !wanted[r.ID] {
						return nil, ErrDenied
					}
					delete(wanted, r.ID)
				}
				return h.projectMany(ctx, a, rows)
			},
		})
	group.Get("", grid.HandlePage)
	group.Get("/data", gridworkflow.APIHandler(grid.HandleData))
	group.Post("/data", gridworkflow.APIHandler(grid.HandleData))
	group.Get("/create", h.app.Guard(p.Create.Domain, p.Create.Action), func(c fiber.Ctx) error {
		if integrationroles.RolePreviewActive(c) {
			return fiber.ErrForbidden
		}
		a, ok := h.app.CurrentActor(c)
		if !ok {
			return fiber.ErrUnauthorized
		}
		if err := h.d.Authorize(c, a, Create, Record[R]{}); err != nil {
			return h.failure(c, err, nil)
		}
		props := h.props(c)
		props["mode"] = string(Create)
		props["values"] = map[string]any{}
		return h.app.Render(c, "crud/Form", props)
	})
	group.Get("/records/:id", func(c fiber.Ctx) error { return h.page(c, false) })
	group.Get("/records/:id/edit",
		h.app.Guard(p.Update.Domain,
			p.Update.Action),
		func(c fiber.Ctx) error {
			return h.page(c,
				true)
		})
	create := host.Command(h.app, p.Create, func(c fiber.Ctx, a host.Actor, raw map[string]json.RawMessage) error {
		var input C
		if err := h.decode(c, raw, Create, &input); err != nil {
			return h.failure(c, err, raw)
		}
		if err := h.d.Authorize(c, a, Create, Record[R]{}); err != nil {
			return h.failure(c, err, raw)
		}
		r, err := h.d.Create(c, a, input)
		if err != nil {
			return h.failure(c, err, raw)
		}
		if !validRecord(r) {
			return h.failure(c, errors.New("invalid create result"), raw)
		}
		h.app.WithFlashSuccess(c, "Record created")
		return h.redirect(c, h.afterWrite(c, a, r))
	})
	group.Post("", create[0], create[1])
	update := host.Command(h.app, p.Update, func(c fiber.Ctx, a host.Actor, raw map[string]json.RawMessage) error {
		var input U
		if err := h.decode(c, raw, Update, &input); err != nil {
			return h.failure(c, err, raw)
		}
		revision, err := revisionFrom(raw)
		if err != nil {
			return h.failure(c, err, raw)
		}
		r, err := h.load(c, a, Update)
		if err != nil {
			return h.failure(c, err, raw)
		}
		if r.Revision != revision {
			return h.failure(c, ErrConflict, raw)
		}
		updated, err := h.d.Update(c, a, r.ID, revision, input)
		if err != nil {
			return h.failure(c, err, raw)
		}
		if !validRecord(updated) || updated.ID != r.ID {
			return h.failure(c, errors.New("invalid update result"), raw)
		}
		h.app.WithFlashSuccess(c, "Record updated")
		return h.redirect(c, h.afterWrite(c, a, updated))
	})
	group.Put("/records/:id", update[0], update[1])
	remove := host.Command(h.app, p.Delete, func(c fiber.Ctx, a host.Actor, raw map[string]json.RawMessage) error {
		return h.mutate(c, a, raw, Delete, h.d.Delete, h.d.BasePath)
	})
	group.Delete("/records/:id", remove[0], remove[1])
	for _, action := range h.d.Actions {
		command := host.Command(h.app, action.Permission, func(c fiber.Ctx, a host.Actor, raw map[string]json.RawMessage) error {
			return h.mutate(c, a, raw, Operation(action.Key), action.Run, h.recordURL(requestID(c)))
		})
		group.Post("/records/:id/actions/"+action.Key, command[0], command[1])
	}
}

func (h *handler[R, C, U]) props(c fiber.Ctx) map[string]any {
	a, _ := h.app.CurrentActor(c)
	can := func(p host.PermissionKey) bool {
		return !integrationroles.RolePreviewActive(c) && h.app.Unwrap().RolesService().AdminCan(c, a.AdminID, p.Domain, p.Action)
	}
	return map[string]any{
		"title": h.d.Title,
		"resource": map[string]any{
			"key":      h.d.Key,
			"basePath": h.d.BasePath,
			"title":    h.d.Title,
			"fields":   h.d.Fields,
			"actions":  h.visibleActions(c),
		},
		"permissions": map[string]bool{
			string(Create): h.canCreate(c),
			string(Update): can(h.d.Permissions.Update),
			string(Delete): can(h.d.Permissions.Delete),
		},
	}
}

func (h *handler[R, C, U]) page(c fiber.Ctx, edit bool) error {
	if edit && integrationroles.RolePreviewActive(c) {
		return fiber.ErrForbidden
	}
	a, ok := h.app.CurrentActor(c)
	if !ok {
		return fiber.ErrUnauthorized
	}
	op := Read
	if edit {
		op = Update
	}
	r, err := h.load(c, a, op)
	if err != nil {
		return h.failure(c, err, nil)
	}
	data, err := h.project(r)
	if err != nil {
		return h.failure(c, err, nil)
	}
	delete(data, "id")
	delete(data, revisionKey)
	actions := []string{}
	if !integrationroles.RolePreviewActive(c) && h.app.Unwrap().RolesService().AdminCan(c,
		a.AdminID,
		h.d.Permissions.Update.Domain,
		h.d.Permissions.Update.Action) && h.d.Authorize(c,
		a,
		Update,
		r) == nil {
		actions = append(actions, "edit")
	}
	if !integrationroles.RolePreviewActive(c) && h.app.Unwrap().RolesService().AdminCan(c,
		a.AdminID,
		h.d.Permissions.Delete.Domain,
		h.d.Permissions.Delete.Action) && h.d.Authorize(c,
		a,
		Delete,
		r) == nil {
		actions = append(actions, string(Delete))
	}
	for _, action := range h.d.Actions {
		if !integrationroles.RolePreviewActive(c) && h.app.Unwrap().RolesService().AdminCan(c,
			a.AdminID,
			action.Permission.Domain,
			action.Permission.Action) && h.d.Authorize(c,
			a,
			Operation(action.Key),
			r) == nil {
			actions = append(actions, action.Key)
		}
	}
	props := h.props(c)
	props["record"] = map[string]any{"id": r.ID, "revision": r.Revision, "data": data, "actions": actions}
	props["values"] = data
	page := "crud/Show"
	if edit {
		page = "crud/Form"
		props["mode"] = string(Update)
	}
	return h.app.Render(c, page, props)
}

func (h *handler[R, C, U]) load(c fiber.Ctx, a host.Actor, op Operation) (Record[R], error) {
	r, err := h.d.Get(c, a, requestID(c))
	if err != nil {
		return r, err
	}
	if !validRecord(r) || r.ID != requestID(c) {
		return r, ErrNotFound
	}
	if err = h.d.Authorize(c, a, Read, r); err != nil {
		return r, err
	}
	if op != Read {
		err = h.d.Authorize(c, a, op, r)
	}
	return r, err
}

func (h *handler[R,
	C,
	U]) mutate(c fiber.Ctx,
	a host.Actor,
	raw map[string]json.RawMessage,
	op Operation,
	run func(context.Context,
		host.Actor,
		string,
		string) error,
	target string,
) error {
	if len(raw) != 1 {
		return h.failure(c, FieldErrors{formKey: {"Only the revision is accepted"}}, nil)
	}
	revision, err := revisionFrom(raw)
	if err != nil {
		return h.failure(c, err, nil)
	}
	r, err := h.load(c, a, op)
	if err != nil {
		return h.failure(c, err, nil)
	}
	if r.Revision != revision {
		return h.failure(c, ErrConflict, nil)
	}
	if err = run(c, a, r.ID, revision); err != nil {
		return h.failure(c, err, nil)
	}
	h.app.WithFlashSuccess(c, "Record action completed")
	if target == h.d.BasePath {
		target = h.listURL(c)
	} else {
		target = h.withReturn(c, target)
	}
	return h.redirect(c, target)
}

func (h *handler[R, C, U]) project(r Record[R]) (projected, error) {
	if !validRecord(r) {
		return nil, errors.New("crud: invalid record identity or revision")
	}
	encoded, err := json.Marshal(r.Data)
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &all); err != nil {
		return nil, err
	}
	types, err := jsonFields[R]()
	if err != nil {
		return nil, err
	}
	out := projected{"id": r.ID, revisionKey: r.Revision}
	for _, f := range h.d.Fields {
		if f.Read {
			if v, ok := all[f.Key]; ok {
				readField := f
				readField.Required = false
				if err := validateFieldValue(readField, types[f.Key], v); err != nil {
					return nil, err
				}
				out[f.Key] = v
			}
		}
	}
	return out, nil
}

func (h *handler[R, C, U]) projectMany(ctx context.Context, a host.Actor, records []Record[R]) ([]projected, error) {
	rows := make([]projected, 0, len(records))
	for _, r := range records {
		if err := h.d.Authorize(ctx, a, Read, r); err != nil {
			return nil, err
		}
		v, err := h.project(r)
		if err != nil {
			return nil, err
		}
		allowed := map[string]bool{}
		for _, entry := range []struct {
			op  Operation
			key host.PermissionKey
		}{{Update, h.d.Permissions.Update}, {Delete, h.d.Permissions.Delete}} {
			allowed[string(entry.op)] = !integrationroles.RolePreviewActive(ctx) && h.d.Authorize(ctx, a, entry.op, r) == nil
		}
		for _, action := range h.d.Actions {
			allowed[action.Key] = !integrationroles.RolePreviewActive(ctx) && h.d.Authorize(ctx, a, Operation(action.Key), r) == nil
		}
		v["_actions"] = allowed
		rows = append(rows, v)
	}
	return rows, nil
}

func validRecord[T any](r Record[T]) bool {
	return r.ID != "" && r.ID != "." && r.ID != ".." && len(r.ID) <= 255 &&
		strings.TrimSpace(r.ID) == r.ID && !strings.ContainsAny(r.ID, ",\r\n\t\x00") &&
		strings.TrimSpace(r.Revision) != "" && len(r.Revision) <= 1024
}

func (h *handler[R, C, U]) recordURL(id string) string {
	return h.d.BasePath + "/records/" + url.PathEscape(id)
}

func (h *handler[R, C, U]) redirect(c fiber.Ctx, target string) error {
	// Extension routes can complete before core Inertia middleware. Persist
	// redirect-surviving state explicitly so form errors cannot become success.
	if err := h.app.PersistFlash(c); err != nil {
		h.app.Logger().ErrorContext(c, "Unable to persist CRUD response", logger.Error(err))
		c.Set("Cache-Control", "no-store")
		// Stop here: the global form error listener may otherwise redirect without
		// persisted errors and incorrectly turn an uncertain write into onSuccess.
		return c.Status(fiber.StatusServiceUnavailable).JSON(map[string]string{
			"message": "Result unknown; check record before retrying",
		})
	}
	return c.Redirect().Status(fiber.StatusSeeOther).To(target)
}

func revisionFrom(raw map[string]json.RawMessage) (string, error) {
	var revision string
	if err := json.Unmarshal(raw[revisionKey],
		&revision); err != nil || strings.TrimSpace(revision) == "" || len(revision) > 1024 {
		return "", FieldErrors{revisionKey: {"A record revision is required"}}
	}
	return revision, nil
}

func (h *handler[R, C, U]) decode(c fiber.Ctx, raw map[string]json.RawMessage, op Operation, target any) error {
	if !strings.HasPrefix(c.Get("Content-Type"), "application/json") {
		return FieldErrors{formKey: {"JSON input is required"}}
	}
	if len(c.Body()) > 64*1024 {
		return FieldErrors{formKey: {"Form exceeds 64 KiB"}}
	}
	types, err := structFields(reflect.TypeOf(target).Elem())
	if err != nil {
		return err
	}
	allowed := map[string]Field{}
	for _, f := range h.d.Fields {
		if op == Create && f.Create || op == Update && f.Update {
			allowed[f.Key] = f
		}
	}
	input := map[string]json.RawMessage{}
	for key, value := range raw {
		if key == revisionKey && op == Update {
			continue
		}
		f, ok := allowed[key]
		if !ok {
			return FieldErrors{formKey: {"Unexpected or read-only field"}}
		}
		if err := validateFieldValue(f, types[key], value); err != nil {
			return FieldErrors{key: {err.Error()}}
		}
		input[key] = value
	}
	for key, f := range allowed {
		if f.Required {
			v, ok := input[key]
			if !ok || string(v) == "null" || string(v) == `""` {
				return FieldErrors{key: {"This field is required"}}
			}
		}
	}
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return FieldErrors{formKey: {"Invalid field value"}}
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return FieldErrors{formKey: {"Invalid form input"}}
	}
	return nil
}

//nolint:gocognit // Explicit route and validation branches keep authorization order visible.
func (h *handler[R, C, U]) failure(c fiber.Ctx, err error, raw map[string]json.RawMessage) error {
	h.app.Logger().ErrorContext(c, "CRUD request failed", logger.Error(err))
	status, message, fields := classifyFailure(err)
	//nolint:nestif // Form errors require an explicit writable-field allowlist before flash persistence.
	if c.Get("X-Inertia") != "" && c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
		if fields == nil {
			fields = FieldErrors{formKey: {message}}
		}
		safe := goinertia.ValidationErrors{}
		for k, v := range fields {
			if k == formKey || k == revisionKey {
				safe["form"] = v
				continue
			}
			for _, f := range h.d.Fields {
				if f.Key == k && (f.Create || f.Update) {
					safe[k] = v
				}
			}
		}
		if len(safe) == 0 {
			safe["form"] = []string{message}
		}
		// Inertia's form helper preserves current input on error redirects. Only
		// declared writable values may enter the server flash fallback.
		old := map[string]any{}
		for _, f := range h.d.Fields {
			if f.Create || f.Update {
				if v, ok := raw[f.Key]; ok {
					var decoded any
					decoder := json.NewDecoder(bytes.NewReader(v))
					if decoder.Decode(&decoded) == nil {
						old[f.Key] = decoded
					}
				}
			}
		}
		h.app.WithFlashOld(c, old)
		h.app.WithValidationErrors(c, safe)
		target := h.listURL(c)
		if c.Method() == fiber.MethodPost && c.Path() == h.d.BasePath {
			target = h.d.BasePath + "/create"
		} else if c.Method() == fiber.MethodPut && !errors.Is(err, ErrUnknown) && c.Params("id") != "" {
			target = h.recordURL(requestID(c)) + "/edit"
		}
		if target != h.listURL(c) {
			target = h.withReturn(c, target)
		}
		return h.redirect(c, target)
	}
	return h.sendFailure(c, status, message, fields)
}

func sanitizeFields(fields []Field, errs FieldErrors) FieldErrors {
	out := FieldErrors{}
	for key, v := range errs {
		if key == formKey || key == revisionKey {
			out[key] = v
		}
		for _, f := range fields {
			if f.Key == key && (f.Create || f.Update) {
				out[key] = v
			}
		}
	}
	return out
}

func (h *handler[R, C, U]) rowCan(ctx context.Context, row projected, op Operation, key host.PermissionKey) bool {
	a, ok := h.app.CurrentActor(ctx)
	if !ok {
		return false
	}
	allowed, _ := row["_actions"].(map[string]bool)
	return !integrationroles.RolePreviewActive(ctx) && allowed[string(op)] && h.app.Unwrap().RolesService().AdminCan(ctx,
		a.AdminID,
		key.Domain,
		key.Action)
}

func (h *handler[R, C, U]) afterWrite(c fiber.Ctx, a host.Actor, r Record[R]) string {
	if h.d.Authorize(c, a, Read, r) != nil {
		return h.listURL(c)
	}
	if !integrationroles.RolePreviewActive(c) && h.app.Unwrap().RolesService().AdminCan(c,
		a.AdminID,
		h.d.Permissions.Update.Domain,
		h.d.Permissions.Update.Action) && h.d.Authorize(c,
		a,
		Update,
		r) == nil {
		return h.withReturn(c, h.recordURL(r.ID)+"/edit")
	}
	return h.withReturn(c, h.recordURL(r.ID))
}

// withReturn carries only a same-resource list location as navigation state.
// It never uses the untrusted value as a redirect destination.
func (h *handler[R, C, U]) validatedReturn(c fiber.Ctx) *url.URL {
	raw := c.Query("returnTo")
	if raw == "" || len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\x00") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil ||
		u.Path != h.d.BasePath || u.Fragment != "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return nil
	}
	if _, err = url.ParseQuery(u.RawQuery); err != nil {
		return nil
	}
	return u
}

func (h *handler[R, C, U]) withReturn(c fiber.Ctx, target string) string {
	u := h.validatedReturn(c)
	if u == nil {
		return target
	}
	returnTo := h.d.BasePath
	if u.RawQuery != "" {
		returnTo += "?" + u.RawQuery
	}
	return target + "?returnTo=" + url.QueryEscape(returnTo)
}

func (h *handler[R, C, U]) listURL(c fiber.Ctx) string {
	u := h.validatedReturn(c)
	if u == nil || u.RawQuery == "" {
		return h.d.BasePath
	}
	return h.d.BasePath + "?" + u.RawQuery
}

func (h *handler[R, C, U]) visibleActions(ctx context.Context) []Action[R] {
	if integrationroles.RolePreviewActive(ctx) {
		return []Action[R]{}
	}
	return h.d.Actions
}

func requestID(c fiber.Ctx) string {
	id, err := url.PathUnescape(c.Params("id"))
	if err != nil {
		return ""
	}
	return id
}

func (h *handler[R, C, U]) canCreate(ctx context.Context) bool {
	if integrationroles.RolePreviewActive(ctx) {
		return false
	}
	a, ok := h.app.CurrentActor(ctx)
	if !ok {
		return false
	}
	p := h.d.Permissions.Create
	return h.app.Unwrap().RolesService().AdminCan(ctx, a.AdminID, p.Domain, p.Action) &&
		h.d.Authorize(ctx, a, Create, Record[R]{}) == nil
}

// apiFailure retains typed resource outcomes without invoking form redirects.
func (h *handler[R, C, U]) apiFailure(c fiber.Ctx, err error) error {
	return h.writeFailure(c, err)
}

func classifyFailure(err error) (int, string, FieldErrors) {
	status, message := fiber.StatusInternalServerError, "Unable to complete resource request"
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		status = fiber.StatusUnprocessableEntity
		message = "Check the form fields"
	case errors.Is(err, ErrNotFound):
		status = fiber.StatusNotFound
		message = "Record not found"
	case errors.Is(err, ErrDenied):
		status = fiber.StatusForbidden
		message = "Record access denied"
	case errors.Is(err, ErrUnknown):
		status = fiber.StatusServiceUnavailable
		message = "Result unknown; check record before retrying"
		fields = FieldErrors{formKey: {message}}
	case errors.Is(err, ErrConflict):
		status = fiber.StatusConflict
		message = "Record changed; reload before saving"
		fields = FieldErrors{revisionKey: {message}}
	}
	return status, message, fields
}

func (h *handler[R, C, U]) writeFailure(c fiber.Ctx, err error) error {
	h.app.Logger().ErrorContext(c, "CRUD data request failed", logger.Error(err))
	status, message, fields := classifyFailure(err)
	return h.sendFailure(c, status, message, fields)
}

func (h *handler[R, C, U]) sendFailure(c fiber.Ctx, status int, message string, fields FieldErrors) error {
	c.Set("Cache-Control", "no-store")
	return c.Status(status).JSON(map[string]any{"message": message, "errors": sanitizeFields(h.d.Fields, fields)})
}
