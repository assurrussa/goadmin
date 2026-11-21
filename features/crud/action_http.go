package crud

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"

	"github.com/gofiber/fiber/v3"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func (h *handler[R, C, U]) parameterAction(action Action[R]) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		actor, ok := h.app.CurrentActor(c)
		if !ok {
			return fiber.ErrUnauthorized
		}
		if integrationroles.RolePreviewActive(c) {
			return action.failure(c, ErrDenied)
		}
		ctx := readContext(c)
		record, err := h.reader().load(ctx, actor, requestID(c))
		if err != nil {
			return action.failure(c, err)
		}
		envelope, err := actionEnvelope(c)
		if err != nil {
			return action.failure(c, err)
		}
		revision, err := revisionFrom(envelope)
		if err != nil {
			return action.failure(c, err)
		}
		// A readable record can have changed into a state where the action is
		// no longer offered. Let the operator refresh that record on conflict.
		if revision != record.Revision {
			return action.failure(c, ErrConflict)
		}
		if err := h.d.Authorize(ctx, actor, Operation(action.Key), record); err != nil {
			return action.failure(c, err)
		}
		values, err := action.values(envelope["params"])
		if err != nil {
			return action.failure(c, err)
		}
		if err := action.input.run(ctx, actor, record.ID, revision, values); err != nil {
			return action.failure(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func actionEnvelope(c fiber.Ctx) (map[string]json.RawMessage, error) {
	if typ, _, _ := mime.ParseMediaType(c.Get("Content-Type")); typ != "application/json" {
		return nil, FieldErrors{formKey: {"JSON input is required"}}
	}
	if len(c.Body()) > 64*1024 {
		return nil, FieldErrors{formKey: {"Form exceeds 64 KiB"}}
	}
	envelope, err := actionObject(c.Body())
	if err != nil {
		return nil, FieldErrors{formKey: {"Invalid form input"}}
	}
	for key := range envelope {
		if key != revisionKey && key != "params" {
			return nil, FieldErrors{formKey: {"Unexpected parameter"}}
		}
	}
	return envelope, nil
}

func (a Action[R]) values(raw json.RawMessage) (map[string]string, error) {
	key := formKey
	if len(a.Params) == 1 {
		key = a.Params[0].Key
	}
	if raw == nil {
		return nil, FieldErrors{key: {"This field is required"}}
	}
	object, err := actionObject(raw)
	if err != nil {
		return nil, FieldErrors{key: {invalidFieldMessage}}
	}
	values := make(map[string]string, len(a.Params))
	problems := FieldErrors{}
	for _, param := range a.Params {
		var text string
		if token, present := object[param.Key]; present {
			delete(object, param.Key)
			if token.Kind() != '"' || !token.IsValid() || json.Unmarshal(token, &text) != nil {
				problems[param.Key] = []string{invalidFieldMessage}
				continue
			}
		}
		text, err = param.Normalize(text)
		if err != nil {
			problems[param.Key] = []string{err.Error()}
			continue
		}
		values[param.Key] = text
	}
	if len(object) > 0 {
		problems[key] = []string{"Unexpected parameter"}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return values, nil
}

// Preserve exact member names and reject repeated keys before binding a DTO.
func actionObject(raw []byte) (map[string]json.RawMessage, error) {
	invalid := errors.New("expected a single JSON object with unique keys")
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if start, err := decoder.Token(); err != nil || start != json.Delim('{') {
		return nil, invalid
	}
	object := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || object[key] != nil {
			return nil, invalid
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, invalid
		}
		object[key] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return nil, invalid
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return nil, invalid
	}
	return object, nil
}

func (a Action[R]) failure(c fiber.Ctx, err error) error {
	status, message, fields := classifyFailure(err)
	safe := FieldErrors{}
	for _, k := range []string{formKey, revisionKey} {
		if v, ok := fields[k]; ok {
			safe[k] = v
		}
	}
	for _, p := range a.Params {
		if v, ok := fields[p.Key]; ok {
			safe[p.Key] = v
		}
	}
	return sendFieldFailure(c, status, message, safe)
}
