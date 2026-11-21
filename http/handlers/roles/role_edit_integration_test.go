//go:build integration

package roles_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

type roleEditorSnapshot struct {
	Revision    string `json:"revision"`
	IDExact     string `json:"idExact"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Permissions []struct {
		Domain string `json:"domain"`
		Action string `json:"action"`
	} `json:"permissions"`
}

type roleEditorResult struct {
	Role   *roleEditorSnapshot `json:"role"`
	Access string              `json:"access"`
}

func roleEditorRequest(
	ctx context.Context, app *fiber.App, method, uri string, data any, inertia bool,
) (int, []byte, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return 0, nil, err
	}
	req := httptest.NewRequestWithContext(ctx, method, uri, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", uri+"/edit")
	if inertia {
		req.Header.Set("X-Inertia", "true")
	}
	response, err := app.Test(req, fiber.TestConfig{Timeout: 8 * time.Second})
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func loadRoleEditor(t *testing.T, app *fiber.App, uri string) roleEditorSnapshot {
	t.Helper()
	status, body, err := roleEditorRequest(t.Context(), app, http.MethodGet, uri+"/edit", nil, true)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, string(body))
	var page struct {
		Props struct {
			Role roleEditorSnapshot `json:"role"`
		} `json:"props"`
	}
	require.NoError(t, json.Unmarshal(body, &page))
	require.Len(t, page.Props.Role.Revision, 64)
	return page.Props.Role
}

func roleEditorWrite(snapshot roleEditorSnapshot, name string, permissions ...string) map[string]any {
	return map[string]any{
		"slug": snapshot.Slug, "name": name, "revision": snapshot.Revision, "permissions": permissions,
	}
}

func editRole(
	t *testing.T, app *fiber.App, uri string, data any, expected int,
) roleEditorResult {
	t.Helper()
	status, body, err := roleEditorRequest(t.Context(), app, http.MethodPut, uri, data, false)
	require.NoError(t, err)
	require.Equal(t, expected, status, string(body))
	var result roleEditorResult
	if expected != http.StatusFound {
		require.NoError(t, json.Unmarshal(body, &result))
	}
	return result
}

func roleEditKeys(t *testing.T, f *roleMutationFixture) (read, update, assign integrationroles.PermissionKey) {
	t.Helper()
	read = integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionRead)
	update = integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate)
	assign = integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionAssign)
	require.NoError(t, f.auth.Roles().EnsurePermissions(t.Context(), []integrationroles.CreatePermissionInput{{Key: update}}))
	keys := []integrationroles.PermissionKey{read, update, assign}
	require.NoError(t, f.auth.Roles().SetRolePermissions(t.Context(), f.operator.ID, keys))
	return read, update, assign
}

func TestRoleEditConflictsWithLegacyWrites(t *testing.T) {
	f := newRoleMutationFixture(t)
	read, _, assign := roleEditKeys(t, f)
	app := f.httpApp(t, f.useCases)
	uri := "/roles/" + strconv.FormatInt(f.first.ID, 10)

	first := loadRoleEditor(t, app, uri)
	require.Equal(t, strconv.FormatInt(f.first.ID, 10), first.IDExact)
	editRole(t, app, uri, map[string]any{"slug": first.Slug, "name": "Legacy client"}, http.StatusFound)
	metadataConflict := editRole(t, app, uri, roleEditorWrite(first, "Stale", read.String()), http.StatusConflict)
	require.NotNil(t, metadataConflict.Role)
	require.Equal(t, "Legacy client", metadataConflict.Role.Name)
	require.Empty(t, metadataConflict.Role.Permissions)

	// Permission replacement does not touch auth_roles.updated_at.
	roleBefore, err := f.auth.Roles().GetRole(t.Context(), f.first.ID)
	require.NoError(t, err)
	status, body, err := roleEditorRequest(t.Context(), app, http.MethodPost, uri+"/permissions",
		map[string]any{"permissions": []string{assign.String()}}, false)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, status, string(body))
	roleAfter, err := f.auth.Roles().GetRole(t.Context(), f.first.ID)
	require.NoError(t, err)
	require.Equal(t, roleBefore.UpdatedAt, roleAfter.UpdatedAt)

	permissionConflict := editRole(t, app, uri, roleEditorWrite(*metadataConflict.Role, "Draft", read.String()), http.StatusConflict)
	require.NotNil(t, permissionConflict.Role)
	require.Equal(t, "Legacy client", permissionConflict.Role.Name)
	require.Len(t, permissionConflict.Role.Permissions, 1)
	require.Equal(t, assign.Action.String(), permissionConflict.Role.Permissions[0].Action)
	draft := roleEditorWrite(*permissionConflict.Role, "Draft", read.String())
	draft["isSystem"] = true
	saved := editRole(t, app, uri, draft, http.StatusOK)
	require.NotNil(t, saved.Role)
	require.Equal(t, "Draft", saved.Role.Name)
	require.Len(t, saved.Role.Permissions, 1)
	require.Equal(t, read.Action.String(), saved.Role.Permissions[0].Action)
	require.Equal(t, *saved.Role, loadRoleEditor(t, app, uri))

	invalid := roleEditorWrite(*saved.Role, "Must roll back", "missing:permission")
	editRole(t, app, uri, invalid, http.StatusUnprocessableEntity)
	require.Equal(t, *saved.Role, loadRoleEditor(t, app, uri))

	downgrade := roleEditorWrite(*saved.Role, "Must preserve system role", read.String())
	downgrade["isSystem"] = false
	editRole(t, app, uri, downgrade, http.StatusUnprocessableEntity)
	require.Equal(t, *saved.Role, loadRoleEditor(t, app, uri))
}

func TestRoleEditWaitsForCanonicalPermissionWriter(t *testing.T) {
	f := newRoleMutationFixture(t)
	read, _, assign := roleEditKeys(t, f)
	app := f.httpApp(t, f.useCases)
	uri := fmt.Sprintf("/roles/%d", f.first.ID)
	original := loadRoleEditor(t, app, uri)

	written, release := make(chan struct{}), make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- f.auth.Runtime().InAuthTransaction(t.Context(), func(txCtx context.Context) error {
			if err := f.auth.Roles().SetRolePermissions(txCtx, f.first.ID, []integrationroles.PermissionKey{assign}); err != nil {
				return err
			}
			close(written)
			select {
			case <-release:
				return nil
			case <-txCtx.Done():
				return txCtx.Err()
			}
		})
	}()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	awaitRoleMutationSignal(t, written)

	type result struct {
		status int
		body   []byte
		err    error
	}
	editDone := make(chan result, 1)
	go func() {
		status, body, err := roleEditorRequest(t.Context(), app, http.MethodPut, uri,
			roleEditorWrite(original, "Must not write", read.String()), false)
		editDone <- result{status: status, body: body, err: err}
	}()
	awaitRoleMutationContention(t, f, nil)
	close(release)
	require.NoError(t, <-writeDone)
	got := <-editDone
	require.NoError(t, got.err)
	require.Equal(t, http.StatusConflict, got.status, string(got.body))
	current := loadRoleEditor(t, app, uri)
	require.Equal(t, original.Name, current.Name)
	require.Len(t, current.Permissions, 1)
	require.Equal(t, assign.Action.String(), current.Permissions[0].Action)
}

func TestRoleEditRevokedPermissionHidesEditorSnapshot(t *testing.T) {
	f := newRoleMutationFixture(t)
	read, update, _ := roleEditKeys(t, f)
	app := f.httpApp(t, f.useCases)
	uri := fmt.Sprintf("/roles/%d", f.operator.ID)
	original := loadRoleEditor(t, app, uri)

	// The update commits even though the actor removes their own edit access.
	saved := editRole(t, app, uri, roleEditorWrite(original, "Read only", read.String()), http.StatusOK)
	require.Equal(t, "list", saved.Access)
	require.Nil(t, saved.Role)
	denied := editRole(t, app, uri, roleEditorWrite(original, "Denied", read.String()), http.StatusForbidden)
	require.Nil(t, denied.Role)
	require.NoError(t, f.auth.Roles().SetRolePermissions(t.Context(), f.operator.ID, []integrationroles.PermissionKey{update}))
	denied = editRole(t, app, uri, roleEditorWrite(original, "Denied", update.String()), http.StatusForbidden)
	require.Nil(t, denied.Role)
	role, err := f.auth.Roles().GetRole(t.Context(), f.operator.ID)
	require.NoError(t, err)
	require.Equal(t, "Read only", role.Name)
}
