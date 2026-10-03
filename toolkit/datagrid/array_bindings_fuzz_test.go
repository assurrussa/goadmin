//nolint:goconst // Independent SQL fixtures intentionally repeat their expected syntax.
package datagrid_test

import (
	"strconv"
	"testing"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// FuzzPublicJSONBArrayBindings checks the operand-binding contract, not whether
// arbitrary values are valid PostgreSQL text or array literals. SQL shape must
// depend only on the operand representation, while its values remain in args.
// Prefix and suffix parameters also catch displaced Dollar placeholder numbers.
func FuzzPublicJSONBArrayBindings(f *testing.F) {
	// Start with the hostile punctuation, JSON-style values, empty/nil cases,
	// and scalar array convention covered by the security regression tests.
	f.Add("x' OR TRUE -- \\ ? , { } \"\nключ", "", -1)
	f.Add("", "", 0)
	f.Add("email", "phone", 7)
	f.Add("{email,phone}", "? $1 ??|", 42)
	f.Add("\x00\xff", "\n\r\t\\\"'", int(^uint(0)>>1))
	f.Add("日本語", "🙂", -int(^uint(0)>>1)-1)

	f.Fuzz(func(t *testing.T, first, second string, number int) {
		for _, operand := range []struct {
			name, question, dollar string
			value                  any
			args                   []any
		}{
			{"strings", "ARRAY[?,?]::text[]", "ARRAY[$2,$3]::text[]", []string{first, second}, []any{first, second}},
			{"JSON strings", "ARRAY[?,?]::text[]", "ARRAY[$2,$3]::text[]", []any{first, second}, []any{first, second}},
			{"integers", "ARRAY[?]::text[]", "ARRAY[$2]::text[]", []int{number}, []any{strconv.Itoa(number)}},
			{"empty strings", "ARRAY[]::text[]", "ARRAY[]::text[]", []string{}, nil},
			{"nil strings", "ARRAY[]::text[]", "ARRAY[]::text[]", []string(nil), nil},
			{"empty JSON array", "ARRAY[]::text[]", "ARRAY[]::text[]", []any{}, nil},
			{"nil JSON array", "ARRAY[]::text[]", "ARRAY[]::text[]", []any(nil), nil},
			{"empty integers", "ARRAY[]::text[]", "ARRAY[]::text[]", []int{}, nil},
			{"nil integers", "ARRAY[]::text[]", "ARRAY[]::text[]", []int(nil), nil},
			{"scalar array literal", "?::text[]", "$2::text[]", first, []any{first}},
			{"null", "?::text[]", "$2::text[]", nil, []any{nil}},
		} {
			for _, operator := range []struct {
				op       datagrid.WhereOperator
				question string
			}{
				{datagrid.OpJSONBHasAnyKey, "??|"},
				{datagrid.OpJSONBHasAllKeys, "??&"},
			} {
				query := datagrid.SQLWherex(
					squirrel.Select("id").From("users").Where("tenant_id = ?", 42),
					datagrid.Filters{Fields: map[string]any{"key": operand.value, "unknown": second}},
					map[string]datagrid.FieldMapping{
						"key": datagrid.NewFieldMappingWithOp("metadata", operator.op),
					},
				).Where("active = ?", true)
				wantArgs := append([]any{42}, operand.args...)
				wantArgs = append(wantArgs, true)
				context := string(operator.op) + "/" + operand.name

				// Compare the complete syntax, rather than checking that an arbitrary
				// value is absent: empty strings or SQL-looking keys can match syntax.
				question, args, err := query.ToSql()
				require.NoError(t, err, context)
				require.Equal(t, "SELECT id FROM users WHERE tenant_id = ? AND metadata "+
					operator.question+" "+operand.question+" AND active = ?", question, context)
				require.Equal(t, wantArgs, args, context)

				dollar, args, err := query.PlaceholderFormat(squirrel.Dollar).ToSql()
				require.NoError(t, err, context)
				require.Equal(t, "SELECT id FROM users WHERE tenant_id = $1 AND metadata "+
					string(operator.op)+" "+operand.dollar+" AND active = $"+strconv.Itoa(len(wantArgs)), dollar, context)
				require.Equal(t, wantArgs, args, context)
			}
		}
	})
}
