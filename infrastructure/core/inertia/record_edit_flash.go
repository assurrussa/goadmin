package inertia

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/assurrussa/goinertia"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
)

const (
	recordEditAttemptHeader = "X-GoAdmin-Record-Attempt"
	recordEditAttemptProp   = "recordEditAttempt"
	recordEditFlashTTL      = 5 * time.Minute
)

type recordAttempt struct {
	key   string
	token string
}

func (a recordAttempt) valid() bool { return a.key != "" }

func recordEditAttempt(c fiber.Ctx, sessionKey string) (recordAttempt, bool) {
	if sessionKey != string(goinertia.ContextKeyProps) || c.Get(goinertia.HeaderInertia) == "" {
		return recordAttempt{}, false
	}
	token := c.Get(recordEditAttemptHeader)
	if token == "" {
		return recordAttempt{}, false
	}

	path := strings.TrimRight(c.Path(), "/")
	switch c.Method() {
	case fiber.MethodGet:
		var edit bool
		path, edit = strings.CutSuffix(path, "/edit")
		if !edit {
			return recordAttempt{}, false
		}
	case fiber.MethodPut:
	default:
		return recordAttempt{}, false
	}
	resource, id, ok := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !ok || strings.Contains(id, "/") || (resource != "users" && resource != "admins") {
		return recordAttempt{}, false
	}

	// Invalid identifiers on a scoped request must never fall back to legacy flash.
	recordID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || recordID <= 0 {
		return recordAttempt{}, true
	}
	parsed, err := uuid.Parse(token)
	if err != nil || parsed.Version() != 4 || parsed.Variant() != uuid.RFC4122 || parsed.String() != token {
		return recordAttempt{}, true
	}
	return recordAttempt{key: resource + "/" + strconv.FormatInt(recordID, 10) + "/" + token, token: token}, true
}

func (b SessionBridge) saveRecordEditFlash(c fiber.Ctx, attempt recordAttempt, value any) error {
	props, ok := value.(map[string]any)
	if !ok || len(props) == 0 {
		return nil
	}
	key, err := b.recordEditStorageKey(c, attempt)
	if err != nil || key == "" {
		return err
	}

	// A map[string]any also keeps the payload incompatible with Fiber's browser
	// session decoder, which expects map[any]any.
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(props); err != nil {
		return err
	}
	return sessioncore.SetRecordEditFlash(c.Context(), b.Store.Storage, key, payload.Bytes(), recordEditFlashTTL)
}

func (b SessionBridge) takeRecordEditFlash(c fiber.Ctx, attempt recordAttempt) (map[string]any, error) {
	key, err := b.recordEditStorageKey(c, attempt)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return map[string]any{}, nil
	}
	payload, err := sessioncore.TakeRecordEditFlash(c.Context(), b.Store.Storage, key)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return map[string]any{}, nil
	}

	var props map[string]any
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&props); err != nil {
		return nil, err
	}
	flash, _ := props[goinertia.ContextPropsFlash].(map[string]string)
	delete(flash, recordEditAttemptProp)
	if flash[goinertia.FlashLevelSuccess.String()] != "" {
		flash[recordEditAttemptProp] = attempt.token
	}
	return props, nil
}

func (b SessionBridge) recordEditStorageKey(c fiber.Ctx, attempt recordAttempt) (string, error) {
	sess, err := b.load(c)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	if sess.Fresh() || sess.Get(sessioncore.AuthAdminKey.String()) == nil {
		return "", nil
	}
	digest := sha256.Sum256([]byte(sess.ID() + "\x00" + attempt.key))
	return sessioncore.RecordEditFlashKeyPrefix + hex.EncodeToString(digest[:]), nil
}
