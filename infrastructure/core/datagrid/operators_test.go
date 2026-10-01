package datagrid_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

func TestOpNewFieldMapping(t *testing.T) {
	mapping := datagrid.NewFieldMapping("test_column")

	assert.Equal(t, "test_column", mapping.Column)
	assert.Equal(t, datagrid.OpEqual, mapping.Operator)
}

func TestOpNewFieldMappingWithOp(t *testing.T) {
	mapping := datagrid.NewFieldMappingWithOp("test_column", datagrid.OpLike)

	assert.Equal(t, "test_column", mapping.Column)
	assert.Equal(t, datagrid.OpLike, mapping.Operator)
}

func TestOpWhereOperators(t *testing.T) {
	// Проверяем что все операторы определены правильно
	assert.Equal(t, datagrid.OpEqual, datagrid.WhereOperator("="))
	assert.Equal(t, datagrid.OpNotEqual, datagrid.WhereOperator("<>"))
	assert.Equal(t, datagrid.OpGreater, datagrid.WhereOperator(">"))
	assert.Equal(t, datagrid.OpGreaterEqual, datagrid.WhereOperator(">="))
	assert.Equal(t, datagrid.OpLess, datagrid.WhereOperator("<"))
	assert.Equal(t, datagrid.OpLessEqual, datagrid.WhereOperator("<="))
	assert.Equal(t, datagrid.OpLike, datagrid.WhereOperator("LIKE"))
	assert.Equal(t, datagrid.OpNotLike, datagrid.WhereOperator("NOT LIKE"))
	assert.Equal(t, datagrid.OpIsNull, datagrid.WhereOperator("IS NULL"))
	assert.Equal(t, datagrid.OpIsNotNull, datagrid.WhereOperator("IS NOT NULL"))
	assert.Equal(t, datagrid.OpIn, datagrid.WhereOperator("IN"))
	assert.Equal(t, datagrid.OpNotIn, datagrid.WhereOperator("NOT IN"))
}
