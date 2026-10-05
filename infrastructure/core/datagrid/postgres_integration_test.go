//go:build integration

package datagrid_test

import (
	"context"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/tests"
)

// Execute the public operator contract against PostgreSQL, rather than merely
// asserting plausible SQL. All data and objects are isolated test fixtures.
func TestDataGridPostgresOperators(t *testing.T) {
	const (
		optionalColumn = "optional"
		labelColumn    = "label"
		jsonColumn     = "doc"
		nestedKey      = "nested"
		tagKey         = "tag"
	)
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "DataGridOperators")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup(ctx)
	})
	conn, err := db.DB().Pool().Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(t.Context(), `CREATE TEMP TABLE datagrid_operator_probe
 (id integer PRIMARY KEY, label text, optional text, doc jsonb);
 INSERT INTO datagrid_operator_probe VALUES
 (1, 'alpha', NULL, '{"tag":"red","nested":{"value":"one"},"count":1}'),
 (2, 'beta', 'yes', '{"tag":"blue","count":2}'), (3, 'gamma', NULL, '{}')`)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, key, column string
		op                datagrid.WhereOperator
		value             any
		want              []int
	}{
		{"equal-value", "x", "id", datagrid.OpEqual, 2, []int{2}},
		{"not-equal", "x", "id", datagrid.OpNotEqual, 2, []int{1, 3}},
		{"greater-value", "x", "id", datagrid.OpGreater, 2, []int{3}},
		{"greater-equal", "x", "id", datagrid.OpGreaterEqual, 2, []int{2, 3}},
		{"less-value", "x", "id", datagrid.OpLess, 2, []int{1}},
		{"less-equal", "x", "id", datagrid.OpLessEqual, 2, []int{1, 2}},
		{"like", "x", labelColumn, datagrid.OpLike, "alph", []int{1}},
		{"not-like", "x", labelColumn, datagrid.OpNotLike, "alph", []int{2, 3}},
		{"null", "x", optionalColumn, datagrid.OpIsNull, true, []int{1, 3}},
		{"not-null", "x", optionalColumn, datagrid.OpIsNotNull, true, []int{2}},
		{"in-integers", "x", "id", datagrid.OpIn, []int{1, 3}, []int{1, 3}},
		{"not-in-integers", "x", "id", datagrid.OpNotIn, []int{1, 3}, []int{2}},
		{"in-strings", "x", labelColumn, datagrid.OpIn, []string{"alpha", "gamma"}, []int{1, 3}},
		{"in-scalar", "x", "id", datagrid.OpIn, 2, []int{2}},
		{"in-null", "x", optionalColumn, datagrid.OpIn, nil, []int{}},
		{"not-in-null", "x", optionalColumn, datagrid.OpNotIn, nil, []int{}},
		{"in-empty", "x", "id", datagrid.OpIn, []int{}, []int{}},
		{"not-in-empty", "x", "id", datagrid.OpNotIn, []int{}, []int{1, 2, 3}},
		{"contains", "x", jsonColumn, datagrid.OpJSONBContains, `{"tag":"red"}`, []int{1}},
		{"contained-by", "x", jsonColumn, datagrid.OpJSONBContainedBy, `{"tag":"blue","count":2}`, []int{2, 3}},
		{"has-key", "x", jsonColumn, datagrid.OpJSONBHasKey, nestedKey, []int{1}},
		{"any-key", "x", jsonColumn, datagrid.OpJSONBHasAnyKey, []string{nestedKey, "missing"}, []int{1}},
		{"all-keys", "x", jsonColumn, datagrid.OpJSONBHasAllKeys, []string{tagKey, nestedKey}, []int{1}},
		{"extract-path", "nested.value", jsonColumn, datagrid.OpJSONBExtractPath, `"one"`, []int{1}},
		{"extract-path-text", "nested.value", jsonColumn, datagrid.OpJSONBExtractPathText, "one", []int{1}},
		{"extract-field", tagKey, jsonColumn, datagrid.OpJSONBExtractField, `"red"`, []int{1}},
		{"extract-field-text", tagKey, jsonColumn, datagrid.OpJSONBExtractFieldText, "red", []int{1}},
		{"path-exists", "x", jsonColumn, datagrid.OpJSONBPathExists, `$.nested.value`, []int{1}},
		{"path-match", "x", jsonColumn, datagrid.OpJSONBPathMatch, `$.count == 2`, []int{2}},
		{"hostile-like", "x", labelColumn, datagrid.OpLike, `' OR true; DROP TABLE datagrid_operator_probe; --`, []int{}},
		{
			"hostile-json-key", "x", jsonColumn, datagrid.OpJSONBHasAnyKey,
			[]string{`'); DROP TABLE datagrid_operator_probe; --`},
			[]int{},
		},
		{"hostile-in", "x", labelColumn, datagrid.OpIn, []string{`alpha') OR true --`}, []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).
				Select("id").From("datagrid_operator_probe").OrderBy("id")
			query, args, err := datagrid.SQLWherex(builder,
				datagrid.Filters{Fields: map[string]any{tc.key: tc.value}},
				map[string]datagrid.FieldMapping{tc.key: datagrid.NewFieldMappingWithOp(tc.column, tc.op)},
			).ToSql()
			require.NoError(t, err)
			rows, err := conn.Query(t.Context(), query, args...)
			require.NoError(t, err, "%s args=%#v", query, args)
			defer rows.Close()
			got := []int{}
			for rows.Next() {
				var id int
				require.NoError(t, rows.Scan(&id))
				got = append(got, id)
			}
			require.NoError(t, rows.Err(), "%s args=%#v", query, args)
			require.Equal(t, tc.want, got)
		})
	}
	var count int
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT count(*) FROM datagrid_operator_probe").Scan(&count))
	require.Equal(t, 3, count)
}
