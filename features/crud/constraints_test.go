//nolint:goconst // Explicit wire values document the SDK and HTTP contract.
package crud //nolint:testpackage // Exercise constructor, projection, and HTTP entry points together.

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
)

func TestFieldConstraintDefinitions(t *testing.T) {
	for _, field := range []Field{
		{Type: "number", Minimum: new(0.0)},
		{Type: "text", MaxLength: new(4)},
		{Type: "text", Create: true, MaxLength: new(0)},
		{Type: "text", Update: true, MaxLength: new(-1)},
		{Type: "number", Update: true, MaxLength: new(1)},
		{Type: "select", Create: true, MaxLength: new(1)},
		{Type: "text", Create: true, Minimum: new(0.0)},
		{Type: "number", Update: true, Minimum: new(math.NaN())},
		{Type: "number", Update: true, Minimum: new(math.Inf(-1))},
		{Type: "number", Update: true, Minimum: new(float64(maxSafeNumber + 1))},
	} {
		d := scalarDefinition[string]("text")
		field.Key, field.Label = "value", "Value"
		d.Fields[0] = field
		_, err := New(d)
		require.ErrorContains(t, err, `field "value":`)
	}
	for _, field := range []Field{{MaxLength: new(2)}, {Minimum: new(0.0)}} {
		d := exampleEvents()
		field.Key, field.Label, field.Type, field.Read = "label", "Label", "text", true
		d.Fields[0] = field
		_, err := NewReadOnly(d)
		require.ErrorContains(t, err, `read-only field "label"`)
	}
	d := definition()
	d.Fields[0].MaxLength = new(200)
	d.Fields = append(d.Fields, Field{Key: "stock", Label: "Stock", Type: "number", Read: true, Update: true, Minimum: new(0.0)})
	_, err := New(d)
	require.NoError(t, err)
	frozen := cloneDefinition(d)
	*d.Fields[0].MaxLength = 300
	*d.Fields[2].Minimum = 8
	require.Equal(t, 200, *frozen.Fields[0].MaxLength)
	require.Zero(t, *frozen.Fields[2].Minimum)
	data, err := json.Marshal(frozen.Fields)
	require.NoError(t, err)
	require.Contains(t, string(data), `"maxLength":200`)
	require.Contains(t, string(data), `"minimum":0`)
	require.Equal(t, 1, strings.Count(string(data), `"minimum"`))
}

func TestValidateFieldConstraints(t *testing.T) {
	name := Field{Type: "textarea", MaxLength: new(200)}
	atLimit := namedText(strings.Repeat("🙂", 200))
	require.NoError(t, name.ValidateConstraints(&atLimit))
	require.EqualError(t, name.ValidateConstraints(string(atLimit)+" "), "Text is too long")
	require.Error(t, name.ValidateConstraints(json.Number("123")))
	require.Error(t, name.ValidateConstraints(200))
	require.Error(t, name.ValidateConstraints((*int)(nil)))
	require.NoError(t, name.ValidateConstraints((*string)(nil)))
	name.Required = true
	require.Error(t, name.ValidateConstraints(nil))

	numeric := Field{Type: "number", Minimum: new(0.3)}
	for _, value := range []any{float32(0.3), 0.3, namedNumber(2), uint16(1), json.Number("3e-1"), (*float32)(nil)} {
		require.NoError(t, numeric.ValidateConstraints(value))
	}
	for _, value := range []any{namedNumber(-1), json.Number("0.299999999999999999999"), uint8(0), 0.2} {
		require.EqualError(t, numeric.ValidateConstraints(value), "Value is below the minimum")
	}
	for _, value := range []any{
		"2", true, (*string)(nil), []int{2}, json.Number("01"), json.Number("1e-999999999"), uint64(1 << 53), math.NaN(),
	} {
		require.Error(t, numeric.ValidateConstraints(value))
	}
	numeric.Minimum = new(0.7)
	require.EqualError(t, numeric.ValidateConstraints(float32(0.7)), "Value is below the minimum")
	numeric.Minimum = new(0.300000005)
	require.NoError(t, numeric.ValidateConstraints(float32(0.3)))
}

func TestWriteConstraintsCollectErrorsAndLeaveHistoricalDataReadable(t *testing.T) {
	d := definition()
	d.Fields[0].MaxLength = new(3)
	d.Fields = append(d.Fields, Field{Key: "stock", Label: "Stock", Type: "number", Update: true, Read: true, Minimum: new(1.0)})
	writes := 0
	update := d.Update
	d.Update = func(ctx context.Context, a host.Actor, id, rev string, input testUpdate) (Record[testRow], error) {
		writes++
		return update(ctx, a, id, rev, input)
	}
	app := mounted(t, d, false, false)
	for _, raw := range []string{`{"name":"Long","stock":0,"_revision":"v1"}`, `{"name":"Long","_revision":"v1"}`} {
		status, body, _ := request(t, app, http.MethodPut, "/people/records/42", raw)
		require.Equal(t, http.StatusUnprocessableEntity, status, body)
		var result struct {
			Errors FieldErrors `json:"errors"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		require.Equal(t, FieldErrors{"name": {"Text is too long"}, "stock": {"Value is below the minimum"}}, result.Errors)
	}
	require.Zero(t, writes)
	status, body, _ := request(t, app, http.MethodGet, "/people/data", "")
	require.Equal(t, http.StatusOK, status, body)
	require.Contains(t, body, `"name":"Alice"`)
	for _, path := range []string{"/people/records/42", "/people/records/42/edit"} {
		props := previewPage(t, app, path)
		record, err := json.Marshal(props["record"])
		require.NoError(t, err)
		require.Contains(t, string(record), `"name":"Alice"`)
	}
	status, body, _ = request(t, app, http.MethodPost, "/people", `{"name":"Bob"}`)
	require.Equal(t, http.StatusSeeOther, status, body) // Stock is update-only.
	status, body, _ = request(t, app, http.MethodPut, "/people/records/42", `{"name":"🙂🙂🙂","stock":1,"_revision":"v1"}`)
	require.Equal(t, http.StatusSeeOther, status, body)
	require.Equal(t, 1, writes)
}

func TestNumericConstraintFollowsRawJSONAndBoundDTO(t *testing.T) {
	d := scalarDefinition[float64]("number")
	d.Fields[0].Minimum = new(0.3)
	app := mountScalar(t, d)
	status, body, _ := request(t, app, http.MethodPost, "/scalars", `{"value":0.299999999999999999999}`)
	require.Equal(t, http.StatusUnprocessableEntity, status, body)
	require.Contains(t, body, "Value is below the minimum")
	status, body, _ = request(t, app, http.MethodPost, "/scalars", `{"value":0.3}`)
	require.Equal(t, http.StatusSeeOther, status, body)

	optional := scalarDefinition[*int]("number")
	optional.Fields[0].Minimum = new(2.0)
	app = mountScalar(t, optional)
	for _, raw := range []string{`{}`, `{"value":null}`} {
		status, body, _ = request(t, app, http.MethodPost, "/scalars", raw)
		require.Equal(t, http.StatusSeeOther, status, body)
	}
}

func TestIntegerDTOReportsFieldForUnrepresentableNumber(t *testing.T) {
	d := scalarDefinition[int32]("number")
	d.Fields[0].Minimum = new(0.0)
	app := mountScalar(t, d)
	for _, req := range []struct{ method, path, body string }{
		{http.MethodPost, "/scalars", `{"value":1.5}`},
		{http.MethodPut, "/scalars/records/42", `{"value":1.5,"_revision":"v1"}`},
		{http.MethodPost, "/scalars", `{"value":2147483648}`},
	} {
		status, body, _ := request(t, app, req.method, req.path, req.body)
		require.Equal(t, http.StatusUnprocessableEntity, status, body)
		var result struct {
			Errors FieldErrors `json:"errors"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		require.Equal(t, FieldErrors{"value": {"Enter a whole number within the supported range"}}, result.Errors)
	}
}

func TestFloat32CallbackReceivesValueAboveMinimum(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		minimum float64
		success bool
	}{
		{`{"value":0.7}`, 0.7, false},
		{`{"value":0.300000005}`, 0.300000005, true},
	} {
		d := scalarDefinition[float32]("number")
		d.Fields[0].Minimum = new(tc.minimum)
		create := d.Create
		called := false
		d.Create = func(ctx context.Context, a host.Actor, dto scalarDTO[float32]) (Record[scalarDTO[float32]], error) {
			called = true
			require.GreaterOrEqual(t, float64(dto.Value), tc.minimum)
			return create(ctx, a, dto)
		}
		status, body, _ := request(t, mountScalar(t, d), http.MethodPost, "/scalars", tc.raw)
		require.Equal(t, tc.success, called)
		if tc.success {
			require.Equal(t, http.StatusSeeOther, status, body)
		} else {
			require.Equal(t, http.StatusUnprocessableEntity, status, body)
			require.Contains(t, body, "Value is below the minimum")
		}
	}
}
