package profile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/assurrussa/goauth/postgres"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

const (
	refreshCookie    = "__Host-goadmin_profile_refresh"
	refreshLifetime  = 2 * time.Minute
	phaseStarted     = "started"
	phaseCandidate   = "candidate"
	phaseClaimed     = "claimed"
	phaseArmed       = "armed"
	completionCookie = "__Host-goadmin_profile_completion"
)

type pending struct {
	Selector         string               `json:"selector,omitempty"`
	Native           string               `json:"native,omitempty"`
	AuthSession      string               `json:"authSession,omitempty"`
	Version          int64                `json:"version,omitempty"`
	Started          time.Time            `json:"started,omitempty"`
	Until            time.Time            `json:"until,omitempty"`
	Phase            string               `json:"phase,omitempty"`
	Original         host.ExternalBinding `json:"original,omitempty"`
	Fresh            host.ExternalBinding `json:"fresh,omitempty"`
	Snapshot         Snapshot             `json:"snapshot,omitempty"`
	CompletionNative string               `json:"completionNative,omitempty"`
	Claim            string               `json:"claim,omitempty"`
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (p pending) valid(selector, native string, now time.Time) bool {
	return p.Selector != "" && p.Selector == digest(selector) && p.Native == native &&
		!p.Started.After(now) && now.Before(p.Until) && !p.Until.After(p.Started.Add(refreshLifetime))
}

// CreateIntent runs only after the mounted self-only CSRF-protected POST.
// The selector is opaque, single-use and hashed at rest. No credential/token is
// stored in this feature. A new intent replaces only an older pending intent.
func (c *Component) CreateIntent(ctx fiber.Ctx, app *host.App, expected string) (string, error) {
	admin, binding, err := c.current(ctx, app)
	if err != nil {
		return "", err
	}
	id := rand.Text()
	now := time.Now().UTC()
	err = c.owned(ctx, binding, func(txctx context.Context, ex postgres.SQLExecutor) error {
		subject := binding.LocalSubject.String()
		if err := originalSessionActive(txctx, ex, pending{AuthSession: admin.AuthSessionID, Original: binding}); err != nil {
			return err
		}
		if err := ensureRecord(txctx, ex, subject); err != nil {
			return err
		}
		r, err := readRecord(txctx, ex, subject)
		if err != nil {
			return err
		}
		if err = nextVersion(r, expected); err != nil {
			return err
		}
		return storePending(txctx, ex, subject, pending{
			Selector: digest(id), Native: host.CSRFSessionBinding(admin.SessionID),
			AuthSession: admin.AuthSessionID, Version: r.Version, Started: now,
			Until: now.Add(refreshLifetime), Phase: phaseStarted, Original: binding,
		})
	})
	if err != nil {
		return "", err
	}
	clearCompletionCookie(ctx)
	ctx.Cookie(&fiber.Cookie{
		Name: refreshCookie, Value: id, Path: "/", Secure: true,
		HTTPOnly: true, SameSite: fiber.CookieSameSiteLaxMode, MaxAge: int(refreshLifetime.Seconds()),
	})
	return id, nil
}

func (c *Component) withIntent(ctx context.Context, selector string, fresh host.ExternalBinding,
	fn func(context.Context, postgres.SQLExecutor, string, record) error,
) error {
	if selector == "" || len(selector) > 128 {
		return ErrDenied
	}
	return c.owned(ctx, fresh, func(ctx context.Context, ex postgres.SQLExecutor) error {
		subject := fresh.LocalSubject.String()
		r, err := readRecord(ctx, ex, subject)
		if err != nil {
			return err
		}
		if r.Pending.Selector != digest(selector) || !samePins(r.Pending.Original, fresh) ||
			r.Version != r.Pending.Version {
			return ErrDenied
		}
		return fn(ctx, ex, subject, r)
	})
}

func originalSessionActive(ctx context.Context, ex postgres.SQLExecutor, p pending) error {
	var active bool
	err := ex.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_sessions s JOIN auth_subjects a ON a.id=s.subject_id
 WHERE s.id::text=$1 AND s.subject_id=$2 AND s.realm='admin' AND s.scope='authenticated'
 AND s.revoked_at IS NULL AND s.expires_at>$3 AND s.security_version=a.security_version)`,
		p.AuthSession, p.Original.LocalSubject, time.Now()).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return ErrDenied
	}
	return nil
}

// Capture accepts ONLY a host-verified committed callback. native is the
// original native cookie's CSRFSessionBinding, not any browser JSON identity.
// It saves a candidate only, never selects/applies it or logs in an admin.
func (c *Component) Capture(ctx context.Context, selector, native string,
	fresh host.ExternalBinding, value Snapshot,
) error {
	value, err := c.avatars.snapshot(value, time.Now())
	if err != nil {
		return err
	}
	if !value.IssuedAt.Equal(fresh.Proof.IssuedAt) || fresh.Generation < fresh.LoginGeneration {
		return ErrDenied
	}
	return c.withIntent(ctx, selector, fresh, func(ctx context.Context, ex postgres.SQLExecutor, subject string, r record) error {
		p := r.Pending
		if p.Phase != phaseStarted || !p.valid(selector, native, time.Now()) ||
			!time.Now().Before(fresh.Deadline) || fresh.LoginGeneration <= 0 ||
			fresh.AuthorityID == p.Original.AuthorityID && fresh.LoginGeneration == p.Original.LoginGeneration {
			return ErrDenied
		}
		if err := originalSessionActive(ctx, ex, p); err != nil {
			return err
		}
		p.Phase, p.Fresh, p.Snapshot = phaseCandidate, fresh, value
		return storePending(ctx, ex, subject, p)
	})
}

// Claim must precede ordinary LoginExternalAdmin. It never admits a cached
// callback or a different native browser, subject, link or provider login.
func (c *Component) Claim(ctx context.Context, selector, native string, fresh host.ExternalBinding) (string, error) {
	claim := rand.Text()
	err := c.withIntent(ctx, selector, fresh, func(ctx context.Context, ex postgres.SQLExecutor, subject string, r record) error {
		p := r.Pending
		if p.Phase != phaseCandidate || !p.valid(selector, native, time.Now()) || !sameProof(p.Fresh, fresh) {
			return ErrDenied
		}
		if err := originalSessionActive(ctx, ex, p); err != nil {
			return err
		}
		p.Phase, p.Claim = phaseClaimed, digest(claim)
		return storePending(ctx, ex, subject, p)
	})
	if err != nil {
		return "", err
	}
	return claim, nil
}

// Arm is called only after successful native login. native is the hash of the
// uniquely server-issued native response cookie, never an incoming browser value.
// It does not apply the snapshot or re-read the old request's native session.
func (c *Component) Arm(ctx fiber.Ctx, selector, claim, native string, fresh host.ExternalBinding) error {
	if err := c.arm(ctx, selector, claim, native, fresh); err != nil {
		return err
	}
	ctx.Cookie(&fiber.Cookie{
		Name: completionCookie, Value: claim, Path: "/", Secure: true,
		HTTPOnly: true, SameSite: fiber.CookieSameSiteLaxMode, MaxAge: int(refreshLifetime.Seconds()),
	})
	return nil
}

func (c *Component) arm(ctx context.Context, selector, claim, native string, fresh host.ExternalBinding) error {
	if len(native) != 64 || claim == "" || len(claim) > 128 {
		return ErrDenied
	}
	if _, err := hex.DecodeString(native); err != nil {
		return ErrDenied
	}
	return c.withIntent(ctx, selector, fresh, func(ctx context.Context, ex postgres.SQLExecutor, subject string, r record) error {
		p := r.Pending
		if p.Phase != phaseClaimed || p.Claim != digest(claim) || !time.Now().Before(p.Until) ||
			p.Native == native || !sameProof(p.Fresh, fresh) {
			return ErrDenied
		}
		p.Phase, p.CompletionNative = phaseArmed, native
		return storePending(ctx, ex, subject, p)
	})
}

// Complete runs in a subsequent authenticated, CSRF-protected request after the
// browser accepted the newly issued native cookie. The claim never enters JS.
func (c *Component) Complete(ctx fiber.Ctx, app *host.App, selector, claim string) error {
	admin, binding, err := c.current(ctx, app)
	if err != nil {
		return err
	}
	return c.complete(ctx, selector, claim, binding, admin.AuthSessionID, host.CSRFSessionBinding(admin.SessionID))
}

func (c *Component) completionReady(ctx fiber.Ctx, app *host.App) bool {
	if ctx.Query("complete") != "1" {
		return false
	}
	admin, binding, err := c.current(ctx, app)
	if err != nil {
		return false
	}
	selector, err := Selector(ctx)
	if err != nil {
		return false
	}
	claim, err := completionClaim(ctx)
	if err != nil || claim == "" {
		return false
	}
	err = c.withIntent(ctx, selector, binding, func(_ context.Context, _ postgres.SQLExecutor, _ string, r record) error {
		p := r.Pending
		if p.Phase != phaseArmed || p.Claim != digest(claim) || !time.Now().Before(p.Until) ||
			p.CompletionNative != host.CSRFSessionBinding(admin.SessionID) || !sameProof(p.Fresh, binding) {
			return ErrDenied
		}
		return nil
	})
	return err == nil
}

func (c *Component) complete(ctx context.Context, selector, claim string,
	binding host.ExternalBinding, authSession, native string,
) error {
	return c.withIntent(ctx, selector, binding, func(
		txctx context.Context, ex postgres.SQLExecutor, subject string, r record,
	) error {
		p := r.Pending
		if err := nextVersion(r, versionString(r.Version)); err != nil {
			return err
		}
		if err := originalSessionActive(txctx, ex, pending{AuthSession: authSession, Original: binding}); err != nil {
			return err
		}
		if p.Phase != phaseArmed || p.CompletionNative == "" || p.CompletionNative != native ||
			p.Claim != digest(claim) || claim == "" ||
			!time.Now().Before(p.Until) || !sameProof(p.Fresh, binding) {
			return ErrDenied
		}
		_, err := ex.ExecContext(txctx, `UPDATE admin_profile_presentations SET version=version+1,
 issuer=$2,external_subject=$3,link_id=$4,client_id=$5,project_id=$6,
 display_name=$7,display_login=$8,avatar_url=$9,issued_at=$10,updated_at=$11,pending='{}'::jsonb WHERE subject_id=$1`,
			subject, binding.Issuer, binding.Subject, binding.LinkID, binding.ClientID, binding.ProjectID,
			p.Snapshot.Name, p.Snapshot.Login, p.Snapshot.AvatarURL, p.Snapshot.IssuedAt, time.Now())
		return err
	})
}

// Cancel is for an already authenticated/CSRF-validated host logout or cancel.
// It cancels only this opaque intent; the selected snapshot remains untouched.
func (c *Component) Cancel(ctx context.Context, selector string) error {
	if request, ok := ctx.(fiber.Ctx); ok {
		clearCompletionCookie(request)
		clearRefreshCookie(request)
	}
	if selector == "" {
		return nil
	}
	if ctx == nil || len(selector) > 128 {
		return ErrDenied
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := c.cfg.Runtime.Database().ExecContext(bounded,
		"UPDATE admin_profile_presentations SET pending='{}'::jsonb WHERE pending->>'selector'=$1", digest(selector))
	return err
}

// Selector reads only this browser's unique opaque refresh cookie.
func Selector(ctx fiber.Ctx) (string, error) {
	headers := make(http.Header)
	for _, raw := range ctx.Request().Header.PeekAll(fiber.HeaderCookie) {
		headers.Add(fiber.HeaderCookie, string(raw))
	}
	return RequestSelector(&http.Request{Header: headers})
}

// RequestSelector has the same duplicate-cookie policy for a host HTTP callback.
func RequestSelector(request *http.Request) (string, error) {
	return uniqueCookie(request, refreshCookie)
}

func uniqueCookie(request *http.Request, name string) (string, error) {
	count, value := 0, ""
	for _, cookie := range request.Cookies() {
		if cookie.Name == name {
			count++
			value = cookie.Value
		}
	}
	if count > 1 || len(value) > 128 {
		return "", ErrDenied
	}
	return value, nil
}

func completionClaim(ctx fiber.Ctx) (string, error) {
	headers := make(http.Header)
	for _, raw := range ctx.Request().Header.PeekAll(fiber.HeaderCookie) {
		headers.Add(fiber.HeaderCookie, string(raw))
	}
	return uniqueCookie(&http.Request{Header: headers}, completionCookie)
}

func clearCompletionCookie(c fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name: completionCookie, Value: "", Path: "/", Secure: true,
		HTTPOnly: true, SameSite: fiber.CookieSameSiteLaxMode, MaxAge: -1,
	})
}

func clearRefreshCookie(c fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name: refreshCookie, Value: "", Path: "/", Secure: true,
		HTTPOnly: true, SameSite: fiber.CookieSameSiteLaxMode, MaxAge: -1,
	})
}
