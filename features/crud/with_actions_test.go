//nolint:goconst // Literal wire keys and values document the public contract.
package crud //nolint:testpackage // Exercise the mounted feature with repository fixtures.

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
)

func commandDefinition() WithActionsDefinition[testRow] {
	d := definition()
	d.Fields[0].Create, d.Fields[0].Update, d.Fields[0].Required = false, false, false
	return WithActionsDefinition[testRow]{
		Key: d.Key, BasePath: d.BasePath, Title: d.Title,
		Permission: d.Permissions.Read, Fields: d.Fields, Columns: d.Columns,
		List: d.List, Get: d.Get, Authorize: d.Authorize,
		Actions: []Action[testRow]{command("publish", Row), command("archive", Detail), command("restore", "")},
	}
}

func command(key string, placement ActionPlacement) Action[testRow] {
	return Action[testRow]{
		Key: key, Label: key, Permission: host.NewPermissionKey("crudtest", host.PermissionAction(key)),
		Placement: placement, Run: func(context.Context, host.Actor, string, string) error { return nil },
	}
}

func mountCommands(t *testing.T, d WithActionsDefinition[testRow], preview bool,
	can func(context.Context, int64, host.PermissionDomain, host.PermissionAction) bool,
) *fiber.App {
	t.Helper()
	_, err := NewWithActions(d)
	require.NoError(t, err)
	return mountResource(t, preview, false, can, func(app *fiber.App, wrapped *host.App) {
		f := feature[testRow, struct{}, struct{}]{d: cloneDefinition(d.definition()), mode: commandResource}
		extension, err := f.Build(host.Menu{})(wrapped)
		require.NoError(t, err)
		permissions := make([]host.PermissionKey, 1, 1+len(d.Actions))
		permissions[0] = d.Permission
		for _, action := range d.Actions {
			permissions = append(permissions, action.Permission)
		}
		require.Equal(t, permissions, f.Descriptor().Permissions)
		for _, h := range extension.Handlers() {
			h.RegisterGroupRoutes(app)
		}
	})
}

func TestCommandResourceValidation(t *testing.T) {
	for name, change := range map[string]func(*WithActionsDefinition[testRow]){
		"no actions": func(d *WithActionsDefinition[testRow]) { d.Actions = nil },
		"duplicate":  func(d *WithActionsDefinition[testRow]) { d.Actions = append(d.Actions, d.Actions[0]) },
		"reserved":   func(d *WithActionsDefinition[testRow]) { d.Actions[0].Key = "delete" },
		"placement":  func(d *WithActionsDefinition[testRow]) { d.Actions[0].Placement = "somewhere" },
		"callback":   func(d *WithActionsDefinition[testRow]) { d.Actions[0].Run = nil },
		"read gate":  func(d *WithActionsDefinition[testRow]) { d.Authorize = nil },
		"field write": func(d *WithActionsDefinition[testRow]) {
			d.Fields[0].Update = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := commandDefinition()
			change(&d)
			_, err := NewWithActions(d)
			require.Error(t, err)
		})
	}
}

func TestCommandResourceRouteAndPlacementContract(t *testing.T) {
	d := commandDefinition()
	app := mountCommands(t, d, false, nil)
	props := previewPage(t, app, "/people/records/42")
	resource, ok := props["resource"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, resource["serverValidateActionParams"])
	require.Equal(t, map[string]any{"create": false, "update": false, "delete": false}, props["permissions"])
	record, ok := props["record"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "v1", record["revision"])
	// A dialog opened from the list rechecks its row-only command on this detail response.
	require.Equal(t, []any{"publish", "archive", "restore"}, record["actions"])
	detailActions, ok := resource["actions"].([]any)
	require.True(t, ok)
	detailKeys := make([]string, 0, len(detailActions))
	for _, item := range detailActions {
		action, ok := item.(map[string]any)
		require.True(t, ok)
		key, ok := action["key"].(string)
		require.True(t, ok)
		detailKeys = append(detailKeys, key)
	}
	require.Equal(t, []string{"archive", "restore"}, detailKeys)

	props = previewPage(t, app, "/people")
	resource, ok = props["resource"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, resource["serverValidateActionParams"])
	require.Len(t, resource["actions"], 2)

	status, body, _ := request(t, app, http.MethodGet, "/people/data", "")
	require.Equal(t, http.StatusOK, status, body)
	for _, wanted := range []string{`"key":"view"`, `"key":"publish"`, `"key":"restore"`, `"_revision":"v1"`, "/table-preferences"} {
		require.Contains(t, body, wanted)
	}
	for _, absent := range []string{`"archive"`, `"key":"edit"`, `"key":"delete"`, "/table-export"} {
		require.NotContains(t, body, absent)
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/create"},
		{http.MethodPost, ""},
		{http.MethodGet, "/records/42/edit"},
		{http.MethodPut, "/records/42"},
		{http.MethodDelete, "/records/42"},
		{http.MethodGet, "/table-export"},
		{http.MethodGet, "/records/42/actions/publish"},
		{http.MethodPost, "/records/42/actions/delete"},
	} {
		status, _, _ = request(t, app, route.method, "/people"+route.path, "")
		require.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, status, route)
	}
	for _, a := range d.Actions {
		status, _, location := request(t, app, http.MethodPost, "/people/records/42/actions/"+a.Key, `{"_revision":"v1"}`)
		require.Equal(t, http.StatusSeeOther, status)
		require.Equal(t, "/people/records/42", location)
	}
}

func TestCommandResourceRetainsReadOnlyListBounds(t *testing.T) {
	app := mountCommands(t, commandDefinition(), false, nil)
	for _, body := range []string{`{"limit":101}`, `{"fields":{"name":["Alice"]}}`} {
		status, _, _ := request(t, app, http.MethodPost, "/people/data", body)
		require.Equal(t, http.StatusBadRequest, status, body)
	}
	status, body, _ := request(t, app, http.MethodPost, "/people/data", `{"limit":100,"fields":{"name":"Alice"}}`)
	require.Equal(t, http.StatusOK, status, body)
}

func TestCommandChecksRunInOrder(t *testing.T) {
	d := commandDefinition()
	var readErr, commandErr, callbackErr error
	var stages []string
	load := d.Get
	d.Get = func(ctx context.Context, a host.Actor, id string) (Record[testRow], error) {
		stages = append(stages, "get")
		return load(ctx, a, id)
	}
	d.Authorize = func(_ context.Context, _ host.Actor, op Operation, _ Record[testRow]) error {
		stages = append(stages, string(op))
		if op == Read {
			return readErr
		}
		return commandErr
	}
	d.Actions[0].Run = func(context.Context, host.Actor, string, string) error {
		stages = append(stages, "run")
		return callbackErr
	}
	runWithReason := func(_ context.Context, _ host.Actor, _, _ string, input reasonInput) error {
		require.Equal(t, "why", input.Reason)
		stages = append(stages, "run")
		return callbackErr
	}
	d.Actions[1] = reasonAction([]ActionParam{reasonParam()}, runWithReason)
	app := mountCommands(t, d, false, nil)
	for _, key := range []string{"publish", "unpublish"} {
		path := "/people/records/42/actions/" + key
		valid := `{"_revision":"v1"}`
		invalid := `{"_revision":"v1","params":{}}`
		if key == "unpublish" {
			valid = `{"_revision":"v1","params":{"reason":" why "}}`
		}
		readErr, commandErr = ErrDenied, ErrDenied
		stages = nil
		status, _, _ := request(t, app, http.MethodPost, path, "{")
		require.Equal(t, http.StatusForbidden, status)
		require.Equal(t, []string{"get", "read"}, stages)

		readErr = nil
		stages = nil
		status, _, _ = request(t, app, http.MethodPost, path, strings.Replace(invalid, "v1", "stale", 1))
		require.Equal(t, http.StatusConflict, status)
		require.Equal(t, []string{"get", "read"}, stages)

		stages = nil
		status, _, _ = request(t, app, http.MethodPost, path, invalid)
		require.Equal(t, http.StatusForbidden, status)
		require.Equal(t, []string{"get", "read", key}, stages)

		commandErr = nil
		stages = nil
		status, _, _ = request(t, app, http.MethodPost, path, invalid)
		require.Equal(t, http.StatusUnprocessableEntity, status)
		require.Equal(t, []string{"get", "read", key}, stages)

		callbackErr = ErrConflict
		stages = nil
		status, _, _ = request(t, app, http.MethodPost, path, valid)
		require.Equal(t, http.StatusConflict, status)
		require.Equal(t, []string{"get", "read", key, "run"}, stages)

		for _, denied := range []bool{false, true} {
			stages = nil
			can := func(_ context.Context, _ int64, _ host.PermissionDomain, action host.PermissionAction) bool {
				return !denied || action != host.PermissionAction(key)
			}
			blocked := mountCommands(t, d, !denied, can)
			status, _, _ = request(t, blocked, http.MethodPost, path, valid)
			require.Equal(t, http.StatusForbidden, status)
			require.Empty(t, stages)
		}
	}
}

func TestActionReturnRequiresExactResourceList(t *testing.T) {
	app := mountCommands(t, commandDefinition(), false, nil)
	for _, raw := range []string{
		"/people?page=3", "/pe%6fple?page=3", `/\host`, "https:/people", "https:people",
		"//other/people", "/elsewhere", "/people#fragment",
	} {
		path := "/people/records/42/actions/publish?returnTo=" + url.QueryEscape(raw)
		status, _, location := request(t, app, http.MethodPost, path, `{"_revision":"v1"}`)
		require.Equal(t, http.StatusSeeOther, status)
		if raw == "/people?page=3" {
			require.Equal(t, "/people/records/42?returnTo=%2Fpeople%3Fpage%3D3", location)
		} else {
			require.Equal(t, "/people/records/42", location)
		}
	}
}
