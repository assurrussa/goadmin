package inertia

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"net/url"
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
	recordEditIDHeader      = "X-GoAdmin-Record-ID"
	recordEditAttemptProp   = "recordEditAttempt"
	crudEditConflictProp    = "crudEditConflict"
	recordEditFlashTTL      = 5 * time.Minute
)

type recordAttempt struct {
	key             string
	token           string
	edit            bool
	conflict        bool
	redirectFromGet bool
}

func (a recordAttempt) valid() bool { return a.key != "" }

type crudFlashScope struct {
	resource        string
	id              string
	page            CRUDRecordPage
	conflict        bool
	redirectFromGet bool
}

// CRUDRecordPage identifies the authorized destination of a CRUD update.
type CRUDRecordPage string

const (
	CRUDRecordIndex CRUDRecordPage = "index"
	CRUDRecordShow  CRUDRecordPage = "show"
	CRUDRecordEdit  CRUDRecordPage = "edit"
)

// ScopeCRUDRecordFlash opts a registered CRUD route into attempt-scoped flash.
// The update and its destination supply the same resource, record, and page.
// Fallback destinations never acknowledge an edit.
func ScopeCRUDRecordFlash(c fiber.Ctx, resource, id string, page CRUDRecordPage, conflict bool) {
	c.Locals(crudFlashScope{}, crudFlashScope{resource: resource, id: id, page: page, conflict: conflict})
}

// ScopeCRUDRecordAccessFallback allows a denied redirect GET to send an error
// to the resource list using the original update attempt.
func ScopeCRUDRecordAccessFallback(c fiber.Ctx, resource, id string) {
	c.Locals(crudFlashScope{}, crudFlashScope{resource: resource, id: id, page: CRUDRecordIndex, redirectFromGet: true})
}

// DiscardCRUDRecordFlash consumes feedback for a destination that failed its
// authorization check without decoding or sharing its fields.
func (b SessionBridge) DiscardCRUDRecordFlash(c fiber.Ctx, resource, id string, page CRUDRecordPage) error {
	ScopeCRUDRecordFlash(c, resource, id, page, false)
	attempt, scoped := recordEditAttempt(c, string(goinertia.ContextKeyProps))
	if !scoped || !attempt.valid() {
		return nil
	}
	key, err := b.recordEditStorageKey(c, attempt)
	if err != nil || key == "" {
		return err
	}
	_, err = sessioncore.TakeRecordEditFlash(c.Context(), b.Store.Storage, key)
	return err
}

// CRUDRecordFlashIndexID reads the opaque record selector for list fallback.
// It identifies flash only and is never used for a record lookup.
func CRUDRecordFlashIndexID(c fiber.Ctx) string {
	if !HasRecordEditAttempt(c) {
		return ""
	}
	raw := c.Get(recordEditIDHeader)
	if len(raw) > 3*255 {
		return ""
	}
	id, _ := url.PathUnescape(raw)
	return id
}

// HasRecordEditAttempt reports whether the request opts into scoped redirects.
func HasRecordEditAttempt(c fiber.Ctx) bool {
	return c.Get(goinertia.HeaderInertia) != "" && c.Get(recordEditAttemptHeader) != ""
}

func recordEditAttempt(c fiber.Ctx, sessionKey string) (recordAttempt, bool) {
	if sessionKey != string(goinertia.ContextKeyProps) || !HasRecordEditAttempt(c) {
		return recordAttempt{}, false
	}
	token := c.Get(recordEditAttemptHeader)

	if scope, ok := c.Locals(crudFlashScope{}).(crudFlashScope); ok {
		if scope.id == "" || len(scope.id) > 255 || strings.ContainsRune(scope.id, '\x00') || !validAttemptToken(token) {
			return recordAttempt{}, true
		}
		key := "crud\x00" + scope.resource + "\x00" + scope.id + "\x00" + string(scope.page) + "\x00" + token
		return recordAttempt{
			key:             key,
			token:           token,
			edit:            scope.page == CRUDRecordEdit,
			conflict:        scope.conflict,
			redirectFromGet: scope.redirectFromGet,
		}, true
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
	if !validAttemptToken(token) {
		return recordAttempt{}, true
	}
	return recordAttempt{key: resource + "/" + strconv.FormatInt(recordID, 10) + "/" + token, token: token, edit: true}, true
}

func validAttemptToken(token string) bool {
	parsed, err := uuid.Parse(token)
	return err == nil && parsed.Version() == 4 && parsed.Variant() == uuid.RFC4122 && parsed.String() == token
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
	if attempt.edit && attempt.conflict {
		flash, ok := props[goinertia.ContextPropsFlash].(map[string]string)
		if !ok {
			flash = map[string]string{}
			props[goinertia.ContextPropsFlash] = flash
		}
		flash[crudEditConflictProp] = "1"
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
	if attempt.edit && (flash[goinertia.FlashLevelSuccess.String()] != "" || flash[crudEditConflictProp] == "1") {
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
