package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/assurrussa/goadmin/host"
)

const linkJSONAccept = "application/json"

// LinkSchema is migrated by the host in the same dedicated schema as Schema.
// Intent identifiers are hashed at rest; no provider tokens are stored here.
func LinkSchema() string {
	return `CREATE TABLE IF NOT EXISTS sso_link_intents (
 id_digest text PRIMARY KEY,
 native_binding text NOT NULL,
 local_subject uuid NOT NULL,
 binding jsonb NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz
);
CREATE INDEX IF NOT EXISTS sso_link_intents_expiry ON sso_link_intents(expires_at);`
}

type intentStore interface {
	create(ctx context.Context, secret, native string, b host.ExternalBinding) error
	consume(ctx context.Context, secret, native string, local goauth.SubjectID) (host.ExternalBinding, error)
}

type postgresIntents struct {
	db    *sql.DB
	table string
}

func intentDigest(secret string) string {
	d := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(d[:])
}

func randomIntent() (string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret[:]), nil
}

func (p postgresIntents) create(ctx context.Context, secret, native string, b host.ExternalBinding) error {
	if err := pruneExpired(ctx, p.db, p.table, "id_digest"); err != nil {
		return err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	//nolint:gosec // table is internal
	insertIntent := `INSERT INTO ` + p.table + ` (id_digest,native_binding,local_subject,binding,expires_at) ` +
		`VALUES ($1,$2,$3,$4,LEAST(clock_timestamp()+interval '5 minutes',$5))`
	_, err = p.db.ExecContext(ctx, insertIntent, intentDigest(secret), native, b.LocalSubject, data, b.Deadline)
	return err
}

func (p postgresIntents) consume(
	ctx context.Context,
	secret, native string,
	local goauth.SubjectID,
) (host.ExternalBinding, error) {
	var data []byte

	consumeIntent := `UPDATE ` + p.table + ` SET consumed_at=clock_timestamp() ` +
		`WHERE id_digest=$1 AND native_binding=$2 AND local_subject=$3 AND consumed_at IS NULL ` +
		`AND expires_at>clock_timestamp() RETURNING binding`
	err := p.db.QueryRowContext(ctx, consumeIntent, intentDigest(secret), native, local).Scan(&data)
	if err != nil {
		return host.ExternalBinding{}, err
	}
	var b host.ExternalBinding
	if err = json.Unmarshal(data, &b); err != nil {
		return host.ExternalBinding{}, err
	}
	if b.LocalSubject != local {
		return host.ExternalBinding{}, ErrDenied
	}
	return b, nil
}

type linkFeature struct {
	service *Runtime
	csrf    *host.CSRFService
}

// LinkFeature mounts GET/POST /sso-link in the authenticated native admin group.
// Local login remains available for this explicit self-link.
// Core and this feature use the same configured CSRF cookie name and supplied
// CSRF service. This adds no roles or membership.
func (s *Runtime) LinkFeature(csrf *host.CSRFService) host.Feature {
	return &linkFeature{service: s, csrf: csrf}
}

func (f *linkFeature) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: "goadmin-sso-link"}
}

func (f *linkFeature) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		if app == nil || f.csrf == nil || f.service == nil || f.service.Client == nil || f.service.intents == nil {
			return nil, ErrDenied
		}
		return host.NewExtension().WithHandlers(&linkHandler{
			service: f.service, admin: app, csrf: f.csrf,
			render: app.Render, verify: f.service.verify, takeReturn: f.service.takeReturn,
		}), nil
	}
}

type linkAdmin interface {
	CurrentActor(ctx context.Context) (host.Actor, bool)
	AdminCSRFRequest(c fiber.Ctx) (host.CSRFRequest, error)
	LocalIdentityLinkEligibility(c fiber.Ctx) (host.IdentityLinkEligibility, error)
	HasExternalAdminIdentity(c fiber.Ctx, issuer, externalSubject string) (bool, error)
	LinkExternalAdminIdentity(c fiber.Ctx, b host.ExternalBinding) (goauth.IdentityLink, error)
	LogoutLocalAdminAuth(c fiber.Ctx) error
}

type linkHandler struct {
	takeReturn func(context.Context, browser.Binding) (string, error)
	verify     func(*http.Request) (browser.Identity, browser.Binding, error)
	render     func(fiber.Ctx, string, map[string]any) error
	service    *Runtime
	admin      linkAdmin
	csrf       *host.CSRFService
}

func (h *linkHandler) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	router.Get("/sso-link", h.page)
	router.Get("/sso-account", h.account)
	// Core authentication, Origin/CSRF and role-preview mutation denial run first.
	router.Post("/sso-link", h.confirm)
	router.Post("/sso-link/reauth", h.reauthenticate)
}

func (h *linkHandler) current(c fiber.Ctx) (goauth.SubjectID, host.CSRFRequest, error) {
	if len(c.Request().Header.PeekAll("Authorization")) != 0 {
		return goauth.SubjectID{}, host.CSRFRequest{}, ErrDenied
	}
	actor, ok := h.admin.CurrentActor(c)
	if !ok {
		return goauth.SubjectID{}, host.CSRFRequest{}, ErrDenied
	}
	local, err := goauth.ParseSubjectID(actor.SubjectID)
	if err != nil {
		return goauth.SubjectID{}, host.CSRFRequest{}, ErrDenied
	}
	csrf, err := h.admin.AdminCSRFRequest(c)
	return local, csrf, err
}

func (h *linkHandler) page(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "strict-origin")
	c.Set("X-Content-Type-Options", "nosniff")
	local, native, err := h.current(c)
	if err != nil {
		return c.SendStatus(http.StatusForbidden)
	}
	eligibility, err := h.admin.LocalIdentityLinkEligibility(c)
	if err != nil {
		return h.failure(c, http.StatusServiceUnavailable, "local_auth_unavailable",
			"Не удалось проверить локальный вход. Попробуйте позже.")
	}
	if eligibility == host.IdentityLinkStale {
		return h.staleFailure(c, native)
	}
	if eligibility != host.IdentityLinkFresh {
		return h.failure(c, http.StatusForbidden, "local_auth_required",
			"Завершите текущий сеанс через меню профиля и войдите локально для привязки.")
	}
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	id, b, err := h.verify(r.WithContext(c.Context()))
	if err != nil {
		if errors.Is(err, browser.ErrUnavailable) {
			return h.failure(c, http.StatusServiceUnavailable, "provider_unavailable",
				"Проверка SSO временно недоступна. Откройте новую форму позже.")
		}
		return h.failure(c, http.StatusUnauthorized, "sso_confirmation_required",
			"Сначала подтвердите аккаунт через SSO, затем откройте привязку снова.")
	}
	if !h.service.matchesSnapshot(id, b) {
		return c.SendStatus(http.StatusForbidden)
	}
	binding, err := buildSnapshot(b, id, local, "", time.Now())
	if err != nil {
		return c.SendStatus(http.StatusForbidden)
	}
	secret, err := randomIntent()
	if err != nil {
		return c.SendStatus(http.StatusServiceUnavailable)
	}
	if err = h.service.intents.create(c.Context(), secret, native.SessionID, binding); err != nil {
		return c.SendStatus(http.StatusServiceUnavailable)
	}
	csrf := c.Cookies(h.service.csrfCookie)
	if _, valid, err := h.csrf.Check(c.Context(), native, csrf); err != nil || !valid {
		token, err := h.csrf.Create(c.Context(), native)
		if err != nil {
			return c.SendStatus(http.StatusServiceUnavailable)
		}
		csrf = token.Token
		c.Cookie(&fiber.Cookie{Name: h.service.csrfCookie, Value: csrf, Path: "/", Secure: true, SameSite: "Lax"})
	}
	return h.render(c, "auth/SSOLinkPage", map[string]any{
		titleProp: "Привязка SSO", "issuer": id.Issuer, "subject": id.Subject,
		"intent": secret, "csrf": csrf, h.service.csrfCookie: csrf,
	})
}

func (h *linkHandler) confirm(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	if len(c.Body()) > 4096 || len(c.Request().Header.PeekAll("Origin")) != 1 ||
		c.Get("Origin") != h.service.Config.Origin ||
		!strings.HasPrefix(c.Get("Content-Type"), "application/x-www-form-urlencoded") ||
		(c.Get("Sec-Fetch-Site") != "" && c.Get("Sec-Fetch-Site") != "same-origin") {
		return h.deny(c, "link_request_invalid", "Запрос привязки отклонён. Откройте форму заново.")
	}
	local, native, err := h.current(c)
	if err != nil {
		return h.deny(c, "link_native_session_invalid", "Локальный вход недоступен. Войдите локально заново.")
	}
	eligibility, err := h.admin.LocalIdentityLinkEligibility(c)
	if err != nil {
		return h.failure(c, http.StatusServiceUnavailable, "local_auth_unavailable",
			"Не удалось проверить локальный вход. Попробуйте позже.")
	}
	if eligibility == host.IdentityLinkStale {
		return h.staleFailure(c, native)
	}
	if eligibility != host.IdentityLinkFresh {
		return h.deny(c, "local_auth_required", "Для привязки войдите локально.")
	}
	args := c.Request().PostArgs()
	for _, name := range []string{"intent", "csrf", "confirm"} {
		if len(args.PeekMulti(name)) != 1 {
			return c.SendStatus(http.StatusBadRequest)
		}
	}
	secret := string(args.Peek("intent"))
	if len(secret) != 43 || string(args.Peek("confirm")) != "link-this-identity" {
		return c.SendStatus(http.StatusBadRequest)
	}
	if _, valid, err := h.csrf.Check(c.Context(), native, string(args.Peek("csrf"))); err != nil || !valid {
		return h.failure(c, http.StatusForbidden, "link_csrf_invalid",
			"Защита формы устарела. Откройте форму привязки заново.")
	}
	// Consume before checking provider state or linking: a failed attempt must
	// obtain a new explicit confirmation, never replay this link operation.
	binding, err := h.service.intents.consume(c.Context(), secret, native.SessionID, local)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return h.failure(c, http.StatusServiceUnavailable, "link_store_unavailable",
				"Не удалось проверить подтверждение. Откройте новую форму позже.")
		}
		return h.failure(c, http.StatusForbidden, "link_intent_invalid",
			"Подтверждение истекло или уже использовано. Откройте форму заново.")
	}
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	_, current, err := h.verify(r.WithContext(c.Context()))
	if err != nil {
		if errors.Is(err, browser.ErrUnavailable) {
			return h.failure(c, http.StatusServiceUnavailable, "provider_unavailable",
				"SSO временно недоступно. Подтверждение использовано: откройте новую форму позже.")
		}
		return h.failure(c, http.StatusForbidden, "sso_confirmation_invalid", "Подтверждение SSO изменилось. Откройте новую форму.")
	}
	pinned, err := sdkBinding(binding)
	if err != nil {
		return h.denyBinding(c, "stored_binding_invalid")
	}
	if reason := sdkBindingDifference(current, pinned); reason != "" {
		return h.denyBinding(c, reason)
	}
	if _, err = h.admin.LinkExternalAdminIdentity(c, binding); err != nil {
		return h.linkFailure(c, native, err)
	}
	// The explicit link grants no session. Continue with a fresh state-bound login.
	target, err := h.takeReturn(c.Context(), current)
	if err != nil {
		return h.failure(c, http.StatusServiceUnavailable, "return_store_unavailable",
			"Учётная запись привязана, но не удалось восстановить путь возврата.")
	}
	next := "/sso?redirectback=" + url.QueryEscape(target)
	if c.Get("Accept") == linkJSONAccept {
		return c.JSON(fiber.Map{"next": next})
	}
	return c.Redirect().Status(http.StatusSeeOther).To(next)
}

func (h *linkHandler) linkFailure(c fiber.Ctx, native host.CSRFRequest, linkErr error) error {
	if errors.Is(linkErr, goauth.ErrIdentityLinkConflict) {
		return h.failure(c, http.StatusConflict, "identity_link_conflict",
			"Эта учётная запись SSO уже привязана. Проверьте существующую привязку.")
	}
	if errors.Is(linkErr, goauth.ErrExplicitIdentityLink) {
		eligibility, checkErr := h.admin.LocalIdentityLinkEligibility(c)
		if checkErr == nil && eligibility == host.IdentityLinkStale {
			return h.staleFailure(c, native)
		}
	}
	if !admissionDenied(linkErr) {
		return h.failure(c, http.StatusServiceUnavailable, "identity_store_unavailable",
			"Хранилище привязок временно недоступно. Откройте новую форму позже.")
	}
	return h.failure(c, http.StatusForbidden, "identity_link_rejected",
		"Не удалось привязать учётную запись. Проверьте локальный аккаунт и существующую привязку.")
}

// account is an authenticated, read-only status endpoint. No provider or
// canonical secrets are returned to the browser.
func (h *linkHandler) account(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	_, _, err := h.current(c)
	if err != nil {
		return c.SendStatus(http.StatusForbidden)
	}
	r, err := adaptor.ConvertRequest(c, true)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	id, binding, verifyErr := h.verify(r.WithContext(c.Context()))
	verified := verifyErr == nil && h.service.matchesSnapshot(id, binding)
	subject := ""
	if verified {
		subject = id.Subject
	}
	linked, err := h.admin.HasExternalAdminIdentity(c, h.service.Config.Issuer, subject)
	if err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"code": "identity_store_unavailable"})
	}
	eligibility, err := h.admin.LocalIdentityLinkEligibility(c)
	if err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"code": "local_auth_unavailable"})
	}
	pending := verified && !linked
	return c.JSON(fiber.Map{"linked": linked, "pending": pending, "localAuth": eligibility})
}

func (h *linkHandler) staleFailure(c fiber.Ctx, native host.CSRFRequest) error {
	const message = "Локальный вход слишком старый для привязки SSO. " +
		"Нажмите «Войти заново», затем подтвердите привязку в новой форме."
	if c.Get("Accept") == linkJSONAccept {
		return h.failure(c, http.StatusForbidden, "local_auth_stale", message)
	}
	token, err := h.csrf.Create(c.Context(), native)
	if err != nil {
		return h.failure(c, http.StatusServiceUnavailable, "local_auth_unavailable",
			"Не удалось подготовить повторный вход. Попробуйте позже.")
	}
	c.Cookie(&fiber.Cookie{Name: h.service.csrfCookie, Value: token.Token, Path: "/", Secure: true, SameSite: "Lax"})
	return h.render(c, "auth/SSOErrorPage", map[string]any{
		titleProp: "Повторный локальный вход", errorCodeProp: "local_auth_stale", messageProp: message,
		"reauthCSRF": token.Token, h.service.csrfCookie: token.Token,
	})
}

// reauthenticate is explicit user intent, not an automatic logout or replay.
func (h *linkHandler) reauthenticate(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	if len(c.Body()) > 1024 || len(c.Request().Header.PeekAll("Origin")) != 1 ||
		c.Get("Origin") != h.service.Config.Origin ||
		!strings.HasPrefix(c.Get("Content-Type"), "application/x-www-form-urlencoded") ||
		(c.Get("Sec-Fetch-Site") != "" && c.Get("Sec-Fetch-Site") != "same-origin") {
		return c.SendStatus(http.StatusForbidden)
	}
	_, native, err := h.current(c)
	if err != nil {
		return c.SendStatus(http.StatusForbidden)
	}
	eligibility, err := h.admin.LocalIdentityLinkEligibility(c)
	if err != nil {
		return c.SendStatus(http.StatusServiceUnavailable)
	}
	if eligibility == host.IdentityLinkNotLocal {
		return c.SendStatus(http.StatusForbidden)
	}
	args := c.Request().PostArgs()
	if len(args.PeekMulti("csrf")) != 1 || len(args.PeekMulti("confirm")) != 1 ||
		string(args.Peek("confirm")) != "reauthenticate-local" {
		return c.SendStatus(http.StatusBadRequest)
	}
	if _, valid, err := h.csrf.Check(c.Context(), native, string(args.Peek("csrf"))); err != nil || !valid {
		return c.SendStatus(http.StatusForbidden)
	}
	if err := h.admin.LogoutLocalAdminAuth(c); err != nil {
		return c.SendStatus(http.StatusServiceUnavailable)
	}
	constNext := "/auth/login?local=1&redirectback=%2Fsso-link"
	if c.Get("Accept") == "application/json" {
		return c.JSON(fiber.Map{"next": constNext})
	}
	return c.Redirect().Status(http.StatusSeeOther).To(constNext)
}

// Compare all original SDK fields, including the fixed deadline. time.Time's
// Go == also compares Location pointers and monotonic clock metadata, which
// are not security properties and need not survive durable serialization.
// The returned enum never contains a value from either binding.
func sdkBindingDifference(a, b browser.Binding) string {
	switch {
	case a.Reference != b.Reference:
		return "reference"
	case a.Generation != b.Generation:
		return "generation"
	case a.LoginGeneration != b.LoginGeneration:
		return "login_generation"
	case !a.Deadline.Equal(b.Deadline):
		return "deadline"
	case a.Issuer != b.Issuer:
		return "issuer"
	case a.Subject != b.Subject:
		return "subject"
	case a.ProjectID != b.ProjectID:
		return "project_id"
	case a.ClientID != b.ClientID:
		return "client_id"
	case a.CentralSessionID != b.CentralSessionID:
		return "central_session_id"
	case a.AuthTime != b.AuthTime:
		return "auth_time"
	case !a.AbsoluteUntil.Equal(b.AbsoluteUntil):
		return "absolute_until"
	default:
		return ""
	}
}

func (h *linkHandler) denyBinding(c fiber.Ctx, reason string) error {
	if c.Get("Accept") != linkJSONAccept {
		return c.SendStatus(http.StatusForbidden)
	}
	return c.Status(http.StatusForbidden).JSON(fiber.Map{
		errorCodeProp: "link_binding_changed", messageProp: "Подтверждение SSO изменилось. Откройте новую форму.",
		"reason": reason,
	})
}

// Keep the original non-JSON denial while making JSON form failures
// distinguishable without returning credentials or identity details.
func (h *linkHandler) deny(c fiber.Ctx, code, message string) error {
	if c.Get("Accept") == linkJSONAccept {
		return h.failure(c, http.StatusForbidden, code, message)
	}
	return c.SendStatus(http.StatusForbidden)
}

func (h *linkHandler) failure(c fiber.Ctx, status int, code, message string) error {
	c.Status(status)
	if c.Get("Accept") == linkJSONAccept {
		return c.JSON(fiber.Map{errorCodeProp: code, messageProp: message})
	}
	return h.render(c, "auth/SSOErrorPage", map[string]any{
		titleProp:     "Не удалось завершить привязку",
		errorCodeProp: code, messageProp: message,
		"linkAvailable": code != "local_auth_required", "startAvailable": code != "local_auth_required",
	})
}

var _ host.Feature = (*linkFeature)(nil)
