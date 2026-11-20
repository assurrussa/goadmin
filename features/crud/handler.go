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
	coreinertia "github.com/assurrussa/goadmin/infrastructure/core/inertia"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/gridworkflow"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type handler[R, C, U any] struct {
	app      *host.App
	d        Definition[R, C, U]
	readOnly bool
}
type projected = map[string]any

func (h *handler[R, C, U]) reader() reader[R, C, U] {
	return reader[R, C, U]{d: &h.d, readOnly: h.readOnly, can: h.app.Unwrap().RolesService().AdminCan}
}

// DataGrid supplies the original request as its repository context.
func readContext(ctx context.Context) context.Context {
	if c, ok := ctx.(fiber.Ctx); ok {
		return requestReadContext{Context: c.Context(), request: c}
	}
	return ctx
}

// requestReadContext combines Fiber's middleware locals with the parent
// context's cancellation, deadlines and request policies. Its lifetime is the
// current HTTP request, just like the context passed to existing host callbacks.
type requestReadContext struct {
	context.Context //nolint:containedctx // Context adapter scopes middleware locals to the request.
	request         fiber.Ctx
}

func (c requestReadContext) Value(key any) any {
	if v := c.request.Value(key); v != nil {
		return v
	}
	return c.Context.Value(key)
}

func (h *handler[R, C, U]) gridRows(records []readRow) []projected {
	rows := make([]projected, 0, len(records))
	for _, record := range records {
		row := record.Data
		row["id"] = record.ID
		if record.Revision != "" {
			row[revisionKey] = record.Revision
			allowed := map[string]bool{string(Update): false, string(Delete): false}
			for _, action := range h.d.Actions {
				allowed[action.Key] = false
			}
			for _, key := range record.Actions {
				if key == editAction {
					key = string(Update)
				}
				allowed[key] = true
			}
			row["_actions"] = allowed
		}
		rows = append(rows, row)
	}
	return rows
}

func (h *handler[R, C, U]) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	p := h.d.Permissions
	group := router.Group(h.d.BasePath, h.recordAttemptGuard(h.app.Guard(p.Read.Domain, p.Read.Action), Read))
	cfg := datagrid.FilterConfig[projected]{
		RoutePath:         h.d.BasePath,
		Entity:            h.d.Key,
		Title:             h.d.Title,
		Columns:           h.d.Columns,
		DefaultSort:       h.d.DefaultSort,
		DefaultOrder:      h.d.DefaultOrder,
		PageSize:          h.d.PageSize,
		Selectable:        !h.readOnly,
		Exportable:        !h.readOnly,
		Refreshable:       true,
		IDKey:             "id",
		Creatable:         !h.readOnly,
		CanViewInPreview:  func(ctx context.Context) bool { return integrationroles.RolePreviewAllows(ctx, p.Read) },
		CanCreate:         h.canCreate,
		APIErrorComponent: h.apiFailure,
	}
	cfg.Actions = []datagrid.Action[projected]{{Key: "view", Label: "Просмотр"}}
	if !h.readOnly {
		add := func(key, label string) {
			cfg.Actions = append(cfg.Actions, datagrid.Action[projected]{
				Key: key, Label: label,
				CanView: func(_ context.Context, row projected) bool {
					allowed, _ := row["_actions"].(map[string]bool)
					if key == editAction {
						return allowed[string(Update)]
					}
					return allowed[key]
				},
			})
		}
		add(editAction, "Редактировать")
		add(string(Delete), "Удалить")
		for _, action := range h.d.Actions {
			add(action.Key, action.Label)
		}
	}
	cfg.Repository = repository[projected](func(ctx context.Context, f datagrid.Filtered) ([]projected, int, error) {
		actor, ok := h.app.CurrentActor(ctx)
		if !ok {
			return nil, 0, ErrDenied
		}
		result, err := h.reader().list(readContext(ctx), actor, datagrid.Filters{
			Page: f.GetPage(), Limit: f.GetLimit(), Search: f.GetSearch(), SortBy: f.GetSortBy(),
			SortOrder: f.GetSortOrder(), Fields: f.GetFields(),
		})
		if err != nil {
			return nil, 0, err
		}
		return h.gridRows(result.Rows), result.Total, nil
	})
	cfg.PageComponent = func(c fiber.Ctx, data datagrid.Response[projected]) error {
		coreinertia.ScopeCRUDRecordFlash(c, h.d.BasePath, coreinertia.CRUDRecordFlashIndexID(c), coreinertia.CRUDRecordIndex, false)
		props, err := h.props(c)
		if err != nil {
			return h.failure(c, err, nil)
		}
		props["data"] = data.ToAPIResponse()
		return h.app.Render(c, "crud/Index", props)
	}
	cfg.ErrorComponent = func(c fiber.Ctx, _ datagrid.ErrorData, err error) error { return h.failure(c, err, nil) }
	grid := datagrid.NewHandler(cfg, h.app.Logger())
	_ = gridworkflow.Register(group,
		grid,
		gridworkflow.Options[projected]{
			Key:           "crud." + h.d.Key,
			DisableExport: h.readOnly,
			Store:         gridworkflow.NewPostgresStore(h.app.Unwrap().GridDB()),
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
				records, err := h.reader().projectList(readContext(ctx), a, rows)
				if err != nil {
					return nil, err
				}
				return h.gridRows(records), nil
			},
		})
	group.Get("", grid.HandlePage)
	group.Get("/data", gridworkflow.APIHandler(grid.HandleData))
	group.Post("/data", gridworkflow.APIHandler(grid.HandleData))
	group.Get("/records/:id", func(c fiber.Ctx) error { return h.page(c, false) })
	if !h.readOnly {
		h.registerMutationRoutes(group)
	}
}

func (h *handler[R, C, U]) registerMutationRoutes(group fiber.Router) {
	p := h.d.Permissions
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
		props, err := h.props(c)
		if err != nil {
			return h.failure(c, err, nil)
		}
		props["mode"] = string(Create)
		props["values"] = map[string]any{}
		return h.app.Render(c, "crud/Form", props)
	})

	group.Get("/records/:id/edit",
		h.recordAttemptGuard(h.app.Guard(p.Update.Domain, p.Update.Action), Update),
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
		return h.redirectUpdate(c, h.afterWrite(c, a, updated), false)
	})
	group.Put("/records/:id", h.recordAttemptGuard(update[0], Update), update[1])
	remove := host.Command(h.app, p.Delete, func(c fiber.Ctx, a host.Actor, raw map[string]json.RawMessage) error {
		return h.mutate(c, a, raw, Delete, h.d.Delete, h.d.BasePath)
	})
	group.Delete("/records/:id", remove[0], remove[1])
	h.registerActionRoutes(group)
}

func (h *handler[R, C, U]) registerActionRoutes(group fiber.Router) {
	for _, action := range h.d.Actions {
		path := "/records/:id/actions/" + action.Key
		if action.input != nil {
			group.Post(path, h.app.Guard(action.Permission.Domain, action.Permission.Action), h.parameterAction(action))
			continue
		}
		command := host.Command(h.app, action.Permission, func(c fiber.Ctx, a host.Actor, raw map[string]json.RawMessage) error {
			return h.mutate(c, a, raw, Operation(action.Key), action.Run, h.recordURL(requestID(c)))
		})
		group.Post(path, command[0], command[1])
	}
}

func (h *handler[R, C, U]) props(c fiber.Ctx) (map[string]any, error) {
	a, _ := h.app.CurrentActor(c)
	description, err := h.reader().describe(readContext(c), a)
	if err != nil {
		return nil, err
	}
	fields := description.Fields
	if !h.readOnly {
		fields = h.d.Fields
	} // Editable forms also need write-only field definitions.
	can := func(p host.PermissionKey) bool {
		return !h.readOnly && !integrationroles.RolePreviewActive(c) &&
			h.app.Unwrap().RolesService().AdminCan(c, a.AdminID, p.Domain, p.Action)
	}
	return map[string]any{
		"title": h.d.Title,
		"resource": map[string]any{
			"key":      description.Key,
			"basePath": h.d.BasePath,
			"title":    description.Title,
			"fields":   fields,
			"actions":  h.visibleActions(c),
		},
		"permissions": map[string]bool{
			string(Create): h.canCreate(c),
			string(Update): can(h.d.Permissions.Update),
			string(Delete): can(h.d.Permissions.Delete),
		},
	}, nil
}

func (h *handler[R, C, U]) page(c fiber.Ctx, edit bool) error {
	if edit && integrationroles.RolePreviewActive(c) {
		return fiber.ErrForbidden
	}
	a, ok := h.app.CurrentActor(c)
	if !ok {
		return fiber.ErrUnauthorized
	}
	r, err := h.pageRecord(c, a, edit)
	if err != nil {
		if c.Method() == fiber.MethodGet && coreinertia.HasRecordEditAttempt(c) &&
			(errors.Is(err, ErrDenied) || errors.Is(err, ErrNotFound)) {
			return h.redirectRecordAccessFailure(c, err)
		}
		return h.failure(c, err, nil)
	}
	props, err := h.props(c)
	if err != nil {
		return h.failure(c, err, nil)
	}
	record := map[string]any{"id": r.ID, "data": r.Data, "actions": r.Actions}
	if !h.readOnly {
		record["revision"] = r.Revision
	}
	props["record"] = record
	props["values"] = r.Data
	page := "crud/Show"
	destination := coreinertia.CRUDRecordShow
	if edit {
		page = "crud/Form"
		destination = coreinertia.CRUDRecordEdit
		props["mode"] = string(Update)
	}
	coreinertia.ScopeCRUDRecordFlash(c, h.d.BasePath, r.ID, destination, false)
	return h.app.Render(c, page, props)
}

func (h *handler[R, C, U]) pageRecord(c fiber.Ctx, a host.Actor, edit bool) (readRow, error) {
	reader := h.reader()
	if !edit {
		return reader.get(readContext(c), a, requestID(c))
	}
	r, err := h.load(c, a, Update)
	if err != nil {
		return readRow{}, err
	}
	return reader.project(readContext(c), a, r, Update, reader.permittedActions(readContext(c), a))
}

func (h *handler[R, C, U]) load(c fiber.Ctx, a host.Actor, op Operation) (Record[R], error) {
	r, err := h.reader().load(readContext(c), a, requestID(c))
	if err != nil {
		return Record[R]{}, err
	}
	if err = h.d.Authorize(readContext(c), a, op, r); err != nil {
		return Record[R]{}, err
	}
	return r, nil
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

func (h *handler[R, C, U]) recordURL(id string) string {
	return h.d.BasePath + "/records/" + url.PathEscape(id)
}

func (h *handler[R, C, U]) recordAttemptGuard(guard fiber.Handler, op Operation) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := guard(c)
		if err == nil || !coreinertia.HasRecordEditAttempt(c) || c.Method() != fiber.MethodPut && c.Method() != fiber.MethodGet {
			return err
		}
		var denied *fiber.Error
		if !errors.As(err, &denied) || denied.Code != fiber.StatusForbidden {
			return err
		}
		if op == Read {
			// The list shares this guard, so following the referer or returning
			// to the list would send the browser to another forbidden route.
			h.discardDeniedRecordFlash(c, coreinertia.CRUDRecordFlashIndexID(c))
			return h.sendFailure(c, fiber.StatusForbidden, "Record access denied", nil)
		}
		return h.redirectRecordAccessFailure(c, ErrDenied)
	}
}

func (h *handler[R, C, U]) redirectRecordAccessFailure(c fiber.Ctx, err error) error {
	_, message, _ := classifyFailure(err)
	h.app.WithValidationErrors(c, goinertia.ValidationErrors{"form": {message}})
	if c.Method() == fiber.MethodGet {
		id := requestID(c)
		h.discardDeniedRecordFlash(c, id)
		coreinertia.ScopeCRUDRecordAccessFallback(c, h.d.BasePath, id)
		return h.redirect(c, h.listURL(c))
	}
	return h.redirectUpdate(c, h.listURL(c), false)
}

func (h *handler[R, C, U]) discardDeniedRecordFlash(c fiber.Ctx, id string) {
	if c.Method() != fiber.MethodGet || id == "" {
		return
	}
	bridge := coreinertia.SessionBridge{Store: h.app.Unwrap().Session()}
	page := h.recordFlashPage(id, c.Path())
	if err := bridge.DiscardCRUDRecordFlash(c, h.d.BasePath, id, page); err != nil {
		h.app.Logger().ErrorContext(c, "Unable to discard denied CRUD response", logger.Error(err))
	}
}

func (h *handler[R, C, U]) redirectUpdate(c fiber.Ctx, target string, conflict bool) error {
	id := requestID(c)
	path, _, _ := strings.Cut(target, "?")
	coreinertia.ScopeCRUDRecordFlash(c, h.d.BasePath, id, h.recordFlashPage(id, path), conflict)
	return h.redirect(c, target)
}

func (h *handler[R, C, U]) recordFlashPage(id, path string) coreinertia.CRUDRecordPage {
	switch path {
	case h.recordURL(id):
		return coreinertia.CRUDRecordShow
	case h.recordURL(id) + "/edit":
		return coreinertia.CRUDRecordEdit
	default:
		return coreinertia.CRUDRecordIndex
	}
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
		if _, ok := allowed[key]; !ok {
			return FieldErrors{formKey: {"Unexpected or read-only field"}}
		}
		input[key] = value
	}
	fieldErrors := FieldErrors{}
	for key, f := range allowed {
		if err := validateFieldValue(f, types[key], input[key]); err != nil {
			fieldErrors[key] = []string{err.Error()}
		}
	}
	if len(fieldErrors) > 0 {
		return fieldErrors
	}
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return FieldErrors{formKey: {invalidFieldMessage}}
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return FieldErrors{formKey: {"Invalid form input"}}
	}
	return validateBoundFields(allowed, reflect.ValueOf(target).Elem())
}

// Binding can supply a zero for an omitted non-pointer or round a numeric
// value to the DTO's precision. Check the value the callback will receive.
func validateBoundFields(allowed map[string]Field, dto reflect.Value) error {
	fieldErrors := FieldErrors{}
	for i := range dto.NumField() {
		field := dto.Type().Field(i)
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		f, writable := allowed[key]
		if !field.IsExported() || !writable || (f.MaxLength == nil && f.Minimum == nil) {
			continue
		}
		if err := f.ValidateConstraints(dto.Field(i).Interface()); err != nil {
			fieldErrors[key] = []string{err.Error()}
		}
	}
	if len(fieldErrors) > 0 {
		return fieldErrors
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
		h.app.WithValidationErrors(c, safe)
		conflict := status == fiber.StatusConflict
		if conflict {
			toast := message
			if c.Method() == fiber.MethodPut && coreinertia.HasRecordEditAttempt(c) {
				toast = "This record has changed. Review your edits."
			}
			h.app.WithFlashError(c, toast)
		}
		target := h.listURL(c)
		if c.Method() == fiber.MethodPost && c.Path() == h.d.BasePath {
			target = h.d.BasePath + "/create"
		} else if c.Method() == fiber.MethodPut {
			lostAccess := coreinertia.HasRecordEditAttempt(c) && (status == fiber.StatusForbidden || status == fiber.StatusNotFound)
			if status != fiber.StatusServiceUnavailable && !lostAccess && requestID(c) != "" {
				target = h.withReturn(c, h.recordURL(requestID(c))+"/edit")
				h.app.WithFlashOld(c, h.readableDraft(raw, Update))
			}
			return h.redirectUpdate(c, target, conflict)
		}
		if target != h.listURL(c) {
			target = h.withReturn(c, target)
			h.app.WithFlashOld(c, h.readableDraft(raw, Create))
		}
		return h.redirect(c, target)
	}
	return h.sendFailure(c, status, message, fields)
}

// The form retains its draft in memory. The server fallback carries only
// scalar values of fields that may be both read and written in this operation.
func (h *handler[R, C, U]) readableDraft(raw map[string]json.RawMessage, op Operation) map[string]any {
	draft := map[string]any{}
	for _, field := range h.d.Fields {
		writable := op == Create && field.Create || op == Update && field.Update
		if !field.Read || !writable {
			continue
		}
		var value any
		if json.Unmarshal(raw[field.Key], &value) == nil {
			switch value.(type) {
			case nil, bool, float64, string:
				draft[field.Key] = value
			}
		}
	}
	return draft
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
	if len(h.d.Actions) == 0 || h.readOnly || integrationroles.RolePreviewActive(ctx) {
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
	if h.readOnly || integrationroles.RolePreviewActive(ctx) {
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
	case errors.Is(err, ErrInvalidQuery):
		status, message = fiber.StatusBadRequest, "Invalid resource list query"
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
	return sendFieldFailure(c, status, message, sanitizeFields(h.d.Fields, fields))
}

func sendFieldFailure(c fiber.Ctx, status int, message string, safe FieldErrors) error {
	c.Set("Cache-Control", "no-store")
	return c.Status(status).JSON(map[string]any{"message": message, "errors": safe})
}
