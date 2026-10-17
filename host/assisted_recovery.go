package host

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html"
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/gofiber/fiber/v3"
)

const (
	PermissionDomainAccountRecovery   PermissionDomain = "account_recovery"
	AssistedRecoveryAdminPath         string           = "/account-recovery"
	AssistedRecoveryCompletePath      string           = "/auth/assisted-recovery"
	AssistedRecoveryOwnerCompletePath string           = "/auth/assisted-recovery/owner"
)

type AssistedRecoveryCommands interface {
	Initiate(ctx context.Context, request goauth.AssistedRecoveryRequest) error
	Complete(ctx context.Context, request goauth.AssistedRecoveryCompletion) error
}

type AssistedRecoveryConfig struct {
	Commands      AssistedRecoveryCommands
	OwnerCommands AssistedRecoveryCommands
	Origin        string
}

// AssistedRecoveryModule is explicit opt-in. It neither grants its dedicated
// permission nor substitutes the ordinary user/admin update permission.
func AssistedRecoveryModule(cfg AssistedRecoveryConfig) Module {
	return DefinedFeatureModule(ModuleDescriptor{
		Key:             "assisted-recovery",
		RouteNamespaces: []string{AssistedRecoveryAdminPath, AssistedRecoveryCompletePath},
	}, &assistedRecoveryFeature{cfg: cfg})
}

func NewLocalAssistedRecovery(adapter *AuthAdapter, origin string) (*goauth.AssistedRecovery, error) {
	if adapter == nil || !adapter.deliveryEnabled {
		return nil, goauth.ErrAssistedRecoveryUnavailable
	}
	policy, err := NewAssistedRecoveryAuthorization(adapter)
	if err != nil {
		return nil, err
	}
	return goauth.NewAssistedRecovery(adapter.Runtime().Runtime, policy, strings.TrimRight(origin, "/")+AssistedRecoveryCompletePath)
}

type AssistedRecoveryAuthorization struct{ runtime *postgres.Runtime }

func NewAssistedRecoveryAuthorization(adapter *AuthAdapter) (*AssistedRecoveryAuthorization, error) {
	if adapter == nil || !adapter.valid() {
		return nil, goauth.ErrAssistedRecoveryUnavailable
	}
	return &AssistedRecoveryAuthorization{runtime: adapter.Runtime()}, nil
}

// AuthorizeAssistedRecovery rechecks the exact dedicated grant on the managed
// auth handle, independently of any super-admin bypass at the UI route. Identity
// evidence is an operator attestation recorded in the mandatory security audit.
func (p *AssistedRecoveryAuthorization) AuthorizeAssistedRecovery(
	ctx context.Context, actor goauth.AuthContext, target goauth.SubjectID, reason, evidence string,
) error {
	if p == nil || p.runtime == nil ||
		actor.Realm != goauth.RealmAdmin ||
		actor.SubjectID.IsZero() ||
		target.IsZero() ||
		actor.SubjectID == target ||
		strings.TrimSpace(reason) == "" ||
		strings.TrimSpace(evidence) == "" {
		return goauth.ErrAssistedRecoveryDenied
	}
	ex, err := p.runtime.SQLExecutor(ctx)
	if err != nil {
		return err
	}
	var allowed bool
	err = ex.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_subject_roles sr
 JOIN auth_role_permissions rp ON rp.role_id=sr.role_id JOIN auth_permissions p ON p.id=rp.permission_id
 WHERE sr.subject_id=$1 AND p.permission_key='account_recovery:create')`, actor.SubjectID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return goauth.ErrAssistedRecoveryDenied
	}
	return nil
}

// AuthorizeOwnerAssistedRecovery is deliberately absent from all HTTP commands.
// Locking membership/role populations prevents a concurrent second owner from
// appearing between the sole-owner check and invitation issuance. Subject-table
// locking also fences activation of an already assigned but disabled owner.
func (p *AssistedRecoveryAuthorization) AuthorizeOwnerAssistedRecovery(ctx context.Context, subject goauth.SubjectID) error {
	if p == nil || p.runtime == nil || subject.IsZero() {
		return goauth.ErrAssistedRecoveryDenied
	}
	ex, err := p.runtime.SQLExecutor(ctx)
	if err != nil {
		return err
	}
	if _, err := ex.ExecContext(ctx,
		`LOCK TABLE auth_subjects,auth_subject_roles,auth_roles,administrations IN SHARE MODE`); err != nil {
		return err
	}
	rows, err := ex.QueryContext(ctx, `SELECT DISTINCT sr.subject_id FROM auth_subject_roles sr
 JOIN auth_roles r ON r.id=sr.role_id JOIN auth_subjects s ON s.id=sr.subject_id
 JOIN administrations a ON a.subject_id=sr.subject_id
 WHERE r.slug='super_admin' AND s.status='active' AND s.retired_at IS NULL AND a.deleted_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	var owner goauth.SubjectID
	for rows.Next() {
		if err := rows.Scan(&owner); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 1 || owner != subject {
		return goauth.ErrAssistedRecoveryDenied
	}
	return nil
}

func (a *App) CurrentLocalAuth(c fiber.Ctx) (goauth.AuthContext, error) {
	if a == nil || a.inner == nil || a.inner.AdminAuth() == nil {
		return goauth.AuthContext{}, goauth.ErrSessionRevoked
	}
	return a.inner.AdminAuth().CurrentLocalAuth(c)
}

type assistedRecoveryFeature struct{ cfg AssistedRecoveryConfig }

func (f *assistedRecoveryFeature) Descriptor() FeatureDescriptor {
	key := NewPermissionKey(PermissionDomainAccountRecovery, PermissionActionCreate)
	return FeatureDescriptor{
		Key: "assisted-recovery", PermissionDefinitions: []PermissionDefinition{
			NewPermissionDefinition(key, "Initiate independently verified account recovery"),
		},
		Menu: Menu{
			Sections: []Section{{Key: "account-recovery", Title: "Security", Items: []Item{{
				Name: "Account recovery", Href: AssistedRecoveryAdminPath, PermissionKey: key,
			}}}},
		},
	}
}

func (f *assistedRecoveryFeature) Build(_ Menu) ExtensionBuilder {
	return func(app *App) (*Extension, error) {
		origin, err := url.Parse(f.cfg.Origin)
		if err != nil || f.cfg.Commands == nil ||
			origin.Host == "" ||
			origin.User != nil ||
			origin.RawQuery != "" ||
			origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") ||
			(origin.Scheme != "https" && (origin.Scheme != "http" || (origin.Hostname() != "localhost" &&
				origin.Hostname() != "127.0.0.1"))) {
			return nil, errors.New("invalid assisted recovery configuration")
		}
		f.cfg.Origin = origin.Scheme +
			"://" +
			origin.Host
		return NewExtension().WithPublicRegister(func(server *fiber.App) {
			server.Get(AssistedRecoveryCompletePath, f.completionPage)
			server.Post(AssistedRecoveryCompletePath, f.complete)
			if f.cfg.OwnerCommands != nil {
				server.Get(AssistedRecoveryOwnerCompletePath, f.completionPage)
				server.Post(AssistedRecoveryOwnerCompletePath, f.complete)
			}
		}).WithRegister(func(server *fiber.App) {
			server.Get(AssistedRecoveryAdminPath, app.Guard(PermissionDomainAccountRecovery, PermissionActionCreate), f.initiationPage)
			server.Post(AssistedRecoveryAdminPath, app.Guard(PermissionDomainAccountRecovery, PermissionActionCreate), f.initiate(app))
		}), nil
	}
}

func (f *assistedRecoveryFeature) initiate(app *App) fiber.Handler {
	return func(c fiber.Ctx) error {
		values, err := f.form(c, []string{"csrf", "subject", "email", "password", "code", "reason", "evidence", "identity_verified"})
		if err != nil || values.Get("identity_verified") != "yes" {
			return fiber.ErrBadRequest
		}
		auth, err := app.CurrentLocalAuth(c)
		if err != nil {
			return fiber.ErrUnauthorized
		}
		subject, err := goauth.ParseSubjectID(values.Get("subject"))
		if err != nil {
			return fiber.ErrBadRequest
		}
		err = f.cfg.Commands.Initiate(c, goauth.AssistedRecoveryRequest{
			Auth: auth, SubjectID: subject,
			NewEmail: values.Get("email"), Password: values.Get("password"), Code: values.Get("code"), Reason: values.Get("reason"),
			EvidenceReference: values.Get("evidence"),
		})
		if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
			return c.Status(fiber.StatusServiceUnavailable).SendString(
				"Recovery outcome is unknown. Do not repeat automatically; ask the service " +
					"owner to reconcile the request.")
		}
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(
				"Recovery could not be initiated. Recheck permission, identity verification " +
					"and fresh credentials.")
		}
		return c.Status(fiber.StatusAccepted).SendString(
			"An invitation was queued. Account credentials change only after successful completion.")
	}
}

func (f *assistedRecoveryFeature) cookieName() string {
	if strings.HasPrefix(f.cfg.Origin, "https://") {
		return "__Host-goadmin_recovery"
	}
	return "goadmin_recovery"
}

func (f *assistedRecoveryFeature) form(c fiber.Ctx, allowed []string) (url.Values, error) {
	c.Set("Cache-Control", "no-store")
	if c.Get("Origin") != f.cfg.Origin || len(c.Body()) > 16*1024 {
		return nil, fiber.ErrBadRequest
	}
	media, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		return nil, fiber.ErrBadRequest
	}
	values, err := url.ParseQuery(string(c.Body()))
	if err != nil {
		return nil, fiber.ErrBadRequest
	}
	for key, entries := range values {
		found := false
		for _, name := range allowed {
			if name == key {
				found = true
			}
		}
		if !found || len(entries) != 1 {
			return nil, fiber.ErrBadRequest
		}
	}
	want, got := c.Cookies(f.cookieName()), values.Get("csrf")
	if len(want) != 43 || len(got) != 43 || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		return nil, fiber.ErrForbidden
	}
	return values, nil
}

func (f *assistedRecoveryFeature) page(c fiber.Ctx, completion bool) error {
	c.Set("Cache-Control", "no-store")
	if c.Get("X-Inertia") == "true" {
		c.Set("X-Inertia-Location", c.Path())
		return c.SendStatus(fiber.StatusConflict)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fiber.ErrServiceUnavailable
	}
	csrf := base64.RawURLEncoding.EncodeToString(raw)
	c.Cookie(&fiber.Cookie{
		Name:  f.cookieName(),
		Value: csrf, Path: "/", HTTPOnly: true, Secure: strings.HasPrefix(f.cfg.Origin, "https://"),
		SameSite: "Strict", MaxAge: 900, Expires: time.Now().Add(15 * time.Minute),
	})
	c.Set("Cache-Control", "no-store")
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("X-Frame-Options", "DENY")
	c.Set("Content-Security-Policy",
		"default-src 'none'; script-src 'nonce-"+csrf+"'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	title, fields, action := "Initiate account recovery", recoveryInitiationFields, AssistedRecoveryAdminPath
	if completion {
		title, fields, action = "Complete account recovery", recoveryCompletionFields, c.Path()
	}
	body := "<!doctype html><html lang=en><meta charset=utf-8><meta name=viewport " +
		"content='width=device-width,initial-scale=1'><title>" +
		title +
		"</title><main><h1>" +
		title +
		"</h1><form method=post action='" +
		action +
		"'><input type=hidden name=csrf value='" +
		html.EscapeString(csrf) +
		"'>" +
		fields +
		"<button type=submit>Continue</button></form></main>"
	if completion {
		body += "<script nonce='" +
			csrf +
			"'>const p=new " +
			"URLSearchParams(location.hash.slice(1));document.querySelector('[name=token" +
			"]').value=p.get('token')||'';history.replaceState(null,'',location.pathname" +
			");</script>"
	}
	body += "</html>"
	c.Type("html")
	return c.SendString(body)
}

func (f *assistedRecoveryFeature) initiationPage(c fiber.Ctx) error { return f.page(c, false) }
func (f *assistedRecoveryFeature) completionPage(c fiber.Ctx) error { return f.page(c, true) }

func (f *assistedRecoveryFeature) complete(c fiber.Ctx) error {
	values, err := f.form(c, []string{"csrf", "token", "password", "password_confirmation", "code"})
	if err != nil || values.Get("password") != values.Get("password_confirmation") {
		return fiber.ErrBadRequest
	}
	commands := f.cfg.Commands
	if c.Path() == AssistedRecoveryOwnerCompletePath {
		commands = f.cfg.OwnerCommands
	}
	if commands == nil {
		return fiber.ErrNotFound
	}
	err = commands.Complete(c, goauth.AssistedRecoveryCompletion{
		Token: values.Get("token"), NewPassword: values.Get("password"), Code: values.Get("code"),
	})
	if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
		return c.Status(fiber.StatusServiceUnavailable).SendString(
			"Recovery outcome is unknown. Do not repeat automatically; contact the " +
				"service owner before requesting another invitation.")
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(
			"Recovery could not be completed. Check the invitation, password and " +
				"existing authenticator or recovery code.")
	}
	return c.SendString(
		"Recovery completed. Sign in with your new email and password. Your " +
			"existing authenticator is unchanged.")
}

const recoveryInitiationFields = `<p>Verify identity independently using established trusted records or a
known channel. A name or a new email address is insufficient.</p>
<p><label>Canonical subject ID <input name=subject required autocomplete=off></label></p>
<p><label>New email <input name=email type=email required autocomplete=off></label></p>
<p><label>Reason <input name=reason required maxlength=512 autocomplete=off></label></p>
<p><label>Independent verification reference (no secrets) <input name=evidence required maxlength=256
autocomplete=off></label></p>
<p><label><input name=identity_verified type=checkbox value=yes required>I independently verified this
person's identity.</label></p>
<p><label>Your current password <input name=password type=password required autocomplete=current-password></label></p>
<p><label>Your authenticator or unused recovery code, if enrolled <input name=code autocomplete=off
maxlength=128></label></p>`

const recoveryCompletionFields = `<p>The invitation verifies your new mailbox. If MFA is enrolled, your
existing authenticator or an unused recovery code is also required. Contact support separately if you lost MFA.</p>
<input name=token type=hidden>
<p><label>New password <input name=password type=password required autocomplete=new-password></label></p>
<p><label>Confirm password <input name=password_confirmation type=password required autocomplete=new-password></label></p>
<p><label>Existing authenticator or unused recovery code <input name=code autocomplete=off maxlength=128></label></p>`
