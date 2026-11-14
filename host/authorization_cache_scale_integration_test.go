//go:build integration

package host //nolint:testpackage // measures public adapter against a large shared identity graph

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"
)

func TestAuthorizationSnapshotScopesHundredThousandCustomers(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	a, err := NewAuthAdapter(AuthAdapterConfig{Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, a.Close()) })
	db := a.Runtime().Database()
	roles, err := a.Runtime().RBAC(nil)
	require.NoError(t, err)
	_, err = roles.UpsertPermission(t.Context(), rbac.Permission{Key: "scale:read"})
	require.NoError(t, err)
	role, err := roles.CreateRole(t.Context(),
		rbac.Role{Slug: "scale-reader", Name: "Scale reader"}, []rbac.PermissionKey{"scale:read"},
	)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO auth_subjects(id,status,created_at,updated_at)
 SELECT md5('scale-'||n)::uuid,'active',now(),now() FROM generate_series(1,100100) n`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO administrations(subject_id,uuid)
 SELECT md5('scale-'||n)::uuid,md5('admin-'||n)::uuid FROM generate_series(1,100) n`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO auth_subject_roles(subject_id,role_id)
 SELECT id,$1 FROM auth_subjects`, role.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `ANALYZE auth_subject_roles; ANALYZE administrations`)
	require.NoError(t, err)
	project := func(ctx context.Context, q postgres.SQLExecutor) ([]goauth.SubjectID, error) {
		rows, e := q.QueryContext(ctx, `SELECT subject_id FROM administrations WHERE deleted_at IS NULL`)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		var subjects []goauth.SubjectID
		for rows.Next() {
			var id goauth.SubjectID
			if e := rows.Scan(&id); e != nil {
				return nil, e
			}
			subjects = append(subjects, id)
		}
		return subjects, rows.Err()
	}
	started := time.Now()
	full, err := a.Runtime().AuthorizationSnapshot(t.Context(), nil)
	require.NoError(t, err)
	fullTime := time.Since(started)
	require.Len(t, full.SubjectRoles, 100100)
	started = time.Now()
	scoped, err := a.Runtime().ScopedAuthorizationSnapshot(t.Context(), project)
	require.NoError(t, err)
	scopedTime := time.Since(started)
	require.Len(t, scoped.SubjectRoles, 100)
	require.Equal(t, full.Roles, scoped.Roles)
	require.Equal(t, full.Permissions, scoped.Permissions)
	empty, err := a.Runtime().ScopedAuthorizationSnapshot(t.Context(),
		func(context.Context, postgres.SQLExecutor) ([]goauth.SubjectID, error) { return nil, nil },
	)
	require.NoError(t, err)
	require.Empty(t, empty.SubjectRoles)
	ids := make([]string, 0, len(scoped.SubjectRoles))
	for _, grant := range scoped.SubjectRoles {
		ids = append(ids, grant.SubjectID.String())
	}
	var planJSON []byte
	err = db.QueryRowContext(t.Context(), `EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON)
 SELECT subject_id,role_id FROM auth_subject_roles
 WHERE subject_id=ANY($1::uuid[]) ORDER BY subject_id,role_id`, ids).Scan(&planJSON)
	require.NoError(t, err)
	var plan any
	require.NoError(t, json.Unmarshal(planJSON, &plan))
	require.NotContains(t, string(planJSON), `"Node Type": "Seq Scan"`, "admin lookup must use the subject index")
	t.Logf("100 admin +100000 customer assignments: full=%s (%d rows); scoped=%s (%d rows); query plan=%s",
		fullTime, len(full.SubjectRoles), scopedTime, len(scoped.SubjectRoles), planJSON)
	cacheAdapter, err := NewAuthAdapter(AuthAdapterConfig{
		Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth,
		AuthorizationCache: &AuthorizationCacheConfig{RefreshInterval: time.Hour, MaxAge: 2 * time.Hour},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cacheAdapter.Close()) })
	checker, err := cacheAdapter.SubjectPermissionChecker(DiscardLogger())
	require.NoError(t, err)
	adminID := scoped.SubjectRoles[0].SubjectID
	var customerID goauth.SubjectID
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT md5('scale-100100')::uuid`).Scan(&customerID))
	for range 1000 {
		require.NoError(t, checker.CheckPermission(t.Context(), adminID.String(), NewPermissionKey("scale", "read")))
		require.ErrorIs(t,
			checker.CheckPermission(t.Context(), customerID.String(), NewPermissionKey("scale", "read")),
			ErrSubjectPermissionDenied,
		)
	}
	allowed, err := roles.Check(t.Context(), customerID, "scale:read")
	require.NoError(t, err)
	require.True(t, allowed)
}
