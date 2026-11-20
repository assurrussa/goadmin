//nolint:goconst // Literal JSON bodies and expected keys document the public wire contract.
package crud //nolint:testpackage // Exercise the mounted SDK, resource metadata and definition snapshot.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
)

type reasonInput struct {
	Reason string `json:"reason,omitempty"`
}

func reasonAction[P any](params []ActionParam, run func(context.Context, host.Actor, string, string, P) error) Action[testRow] {
	return ActionWithParams(Action[testRow]{
		Key: "unpublish", Label: "Unpublish", Permission: host.NewPermissionKey("crudtest", "unpublish"),
	}, params, run)
}

func reasonParam() ActionParam {
	return ActionParam{Key: "reason", Label: "Reason", Type: "textarea", Required: true, TrimSpace: true, MaxLength: new(500)}
}

func checkInputType[P any](t *testing.T, params ...ActionParam) error {
	t.Helper()
	d := definition()
	d.Actions = []Action[testRow]{reasonAction(params, func(context.Context, host.Actor, string, string, P) error { return nil })}
	_, err := New(d)
	return err
}

func TestActionWithParamsValidatesBinding(t *testing.T) {
	p := reasonParam()
	require.NoError(t, checkInputType[reasonInput](t, p))
	require.Error(t, checkInputType[string](t, p))
	require.Error(t, checkInputType[struct{ Reason string }](t, p))
	require.Error(t, checkInputType[struct {
		Reason int `json:"reason"`
	}](t, p))
	require.Error(t, checkInputType[struct {
		Reason *string `json:"reason"`
	}](t, p))
	require.Error(t, checkInputType[struct {
		Reason string `json:"reason,string"`
	}](t, p))
	require.Error(t, checkInputType[struct {
		Reason string `json:"Reason"` //nolint:tagliatelle // Deliberate mismatched DTO key.
	}](t, p))
	require.Error(t, checkInputType[struct {
		Reason string `json:"reason"`
		Other  string `json:"other"`
	}](t, p))
	require.Error(t, checkInputType[reasonInput](t))
	require.Error(t, checkInputType[reasonInput](t, p, p))
	for _, invalid := range []ActionParam{
		{Key: "reason", Label: "Reason", Type: "number"},
		{Key: "reason", Label: "Reason", Type: "textarea", MaxLength: new(0)},
		{Key: "reason", Type: "textarea"},
		{Key: "constructor", Label: "Reason", Type: "textarea"},
	} {
		require.Error(t, checkInputType[reasonInput](t, invalid))
	}
	d := definition()
	a := reasonAction[reasonInput]([]ActionParam{p}, nil)
	d.Actions = []Action[testRow]{a}
	_, err := New(d)
	require.Error(t, err)
	a = reasonAction([]ActionParam{p}, func(context.Context, host.Actor, string, string, reasonInput) error { return nil })
	a.input.dto = reflect.StructOf([]reflect.StructField{
		{Name: "Reason", Type: reflect.TypeFor[string](), Tag: `json:"reason"`},
		{Name: "Other", Type: reflect.TypeFor[string](), Tag: `json:"reason,omitempty"`},
	})
	d.Actions[0] = a
	_, err = New(d)
	require.Error(t, err)
	a.Run = func(context.Context, host.Actor, string, string) error { return nil }
	a.input = nil
	d.Actions[0] = a
	_, err = New(d)
	require.Error(t, err)
}

func TestActionWithParamsNormalizesAndFreezesMetadata(t *testing.T) {
	p := reasonParam()
	raw := "\u0085 \n A  B\n\tC\ufeff \u00a0"
	want := "A  B\n\tC\ufeff"
	bound := strings.Repeat("🙂", 500)
	got, err := p.Normalize(raw)
	require.NoError(t, err)
	require.Equal(t, want, got)
	for _, value := range []string{"\u0085", bound + "s", "NUL\x00reason", string([]byte{0xff})} {
		_, err = p.Normalize(value)
		require.Error(t, err)
	}
	d := definition()
	var values []string
	params := []ActionParam{p}
	run := func(_ context.Context, actor host.Actor, id, rev string, input reasonInput) error {
		require.Equal(t, int64(9), actor.AdminID)
		require.Equal(t, "42", id)
		require.Equal(t, "v1", rev)
		values = append(values, input.Reason)
		return nil
	}
	d.Actions = []Action[testRow]{reasonAction(params, run)}
	app := mounted(t, d, false, false)
	params[0].Key = "changed"
	*p.MaxLength = 2
	props := previewPage(t, app, "/people/records/42")
	metadata, err := json.Marshal(props["resource"])
	require.NoError(t, err)
	require.Contains(t, string(metadata), `"key":"reason"`)
	require.Contains(t, string(metadata), `"maxLength":500`)
	require.Contains(t, string(metadata), `"trimSpace":true`)
	for _, value := range []string{raw, bound} {
		b, err := json.Marshal(map[string]any{"_revision": "v1", "params": map[string]string{"reason": value}})
		require.NoError(t, err)
		status, body, location := request(t, app, http.MethodPost, "/people/records/42/actions/unpublish", string(b))
		require.Equal(t, http.StatusNoContent, status, body)
		require.Empty(t, body)
		require.Empty(t, location)
	}
	status, body, _ := request(t, app, http.MethodPost, "/people/records/42/actions/unpublish",
		`{"_revision":"v1","params":{"reason":"\ud83d\ude42�\\ud800"}}`)
	require.Equal(t, http.StatusNoContent, status, body)
	require.Equal(t, []string{want, bound, `🙂�\ud800`}, values)
}

func TestParameterizedActionStrictHTTP(t *testing.T) {
	d := definition()
	calls := 0
	run := func(context.Context, host.Actor, string, string, reasonInput) error {
		calls++
		return nil
	}
	d.Actions = []Action[testRow]{reasonAction([]ActionParam{reasonParam()}, run)}
	app := mounted(t, d, false, false)
	for _, tc := range []struct{ raw, key string }{
		{`{"_revision":"v1"}`, "reason"},
		{`{"_revision":"v1","params":null}`, "reason"},
		{`{"_revision":"v1","params":[]}`, "reason"},
		{`{"_revision":"v1","params":{}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":null}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":22}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":{}}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":" \u0085\u00a0 "}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"\u0000"}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"\ud800"}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"\udc00"}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"` + string([]byte{0xff}) + `"}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"` + strings.Repeat("🙂", 501) + `"}}`, "reason"},
		{`{"_revision":"v1","params":{"Reason":"private text"}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"private text","unknown":1}}`, "reason"},
		{`{"_revision":"v1","params":{"reason":"first","reason":"second"}}`, "reason"},
		{`{"_revision":"v1","Params":{"reason":"private text"}}`, formKey},
		{`{"_revision":"v1","unknown":true,"params":{"reason":"private text"}}`, formKey},
		{`{"_revision":"v1","_revision":"v2","params":{"reason":"private text"}}`, formKey},
		{`{"_revision":"v1"}{}`, formKey},
		{`[]`, formKey},
		{`{"params":{"reason":"private text"}}`, revisionKey},
	} {
		status, body, _ := request(t, app, http.MethodPost, "/people/records/42/actions/unpublish", tc.raw)
		require.Equal(t, http.StatusUnprocessableEntity, status, body)
		var result struct {
			Errors FieldErrors `json:"errors"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		require.Len(t, result.Errors, 1)
		require.Contains(t, result.Errors, tc.key)
		require.NotContains(t, body, "private text")
	}
	require.Zero(t, calls)
}

func TestParameterizedActionRechecksReadRevisionAndPermission(t *testing.T) {
	const path = "/people/records/42/actions/unpublish"
	const input = `{"_revision":"v1","params":{"reason":"private text"}}`
	d := definition()
	var result error
	var calls int
	run := func(context.Context, host.Actor, string, string, reasonInput) error {
		calls++
		return result
	}
	d.Actions = []Action[testRow]{reasonAction([]ActionParam{reasonParam()}, run)}
	app := mounted(t, d, false, false)
	for _, tc := range []struct {
		err    error
		status int
	}{
		{ErrConflict, 409},
		{ErrUnknown, 503},
		{ErrDenied, 403},
		{ErrNotFound, 404},
		{errors.New("private text"), 500},
		{FieldErrors{"secret": {"private text"}, "reason": {"Safe validation message"}}, 422},
	} {
		result = tc.err
		status, body, _ := request(t, app, http.MethodPost, path, input)
		require.Equal(t, tc.status, status, body)
		require.NotContains(t, body, "private text")
		if tc.status == 422 {
			require.Contains(t, body, "Safe validation message")
		}
	}
	calls = 0
	result = nil
	d.Authorize = func(_ context.Context, _ host.Actor, op Operation, _ Record[testRow]) error {
		if op == "unpublish" {
			return ErrDenied
		}
		return nil
	}
	app = mounted(t, d, false, false)
	status, _, _ := request(t, app, http.MethodPost, path, input)
	require.Equal(t, 403, status)
	stale := strings.Replace(input, "v1", "stale", 1)
	status, _, _ = request(t, app, http.MethodPost, path, stale)
	require.Equal(t, 409, status)
	d.Authorize = func(context.Context, host.Actor, Operation, Record[testRow]) error { return ErrDenied }
	status, _, _ = request(t, mounted(t, d, false, false), http.MethodPost, path, stale)
	require.Equal(t, 403, status)
	d.Authorize = definition().Authorize
	for _, guard := range []struct{ preview, denied bool }{{true, false}, {false, true}} {
		status, _, _ = request(t, mounted(t, d, guard.preview, guard.denied), http.MethodPost, path, input)
		require.Equal(t, 403, status)
	}
	require.Zero(t, calls)
}
