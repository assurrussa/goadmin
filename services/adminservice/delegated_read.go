package adminservice

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

var (
	ErrReadSessionEnded  = errors.New("delegated read session ended")
	ErrReadSessionDenied = errors.New("delegated read session denied")
)

// ReadSessionBinding is an opaque in-process reference to one login.
// The Fiber storage key also acts as a browser cookie. Only its authenticated
// ciphertext leaves the service. Access-token refresh keeps the pin. Replacing
// the browser cookie or restarting the service requires fresh approval.
type ReadSessionBinding struct {
	storageKey string
	loginID    string
	subject    goauth.SubjectID
	adminID    int64
	external   *ExternalBinding
}

type delegatedReadPolicy struct {
	seal         func() (cipher.AEAD, error)
	isSuperAdmin func(context.Context, int64) bool
	roles        integrationroles.RolePreviewReader
}

func WithDelegatedReadPolicy(owner func(context.Context, int64) bool, roles integrationroles.RolePreviewReader) Option {
	return func(s *Service) error {
		if owner == nil || roles == nil {
			return errors.New("delegated reads require role preview authority")
		}
		s.delegatedReads = &delegatedReadPolicy{sync.OnceValues(newReadBindingSeal), owner, roles}
		return nil
	}
}

func newReadBindingSeal() (cipher.AEAD, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// BindDelegatedRead receives the middleware-authenticated identity, including
// the introspected login ID. Verify the login again before issuing a pin.
func (s *Service) BindDelegatedRead(ctx context.Context, admin *models.SessionAdmin) (ReadSessionBinding, error) {
	return s.bindDelegatedRead(ctx, admin, false)
}

// BindDelegatedReadWithExternal opts this approval into external-session checks.
func (s *Service) BindDelegatedReadWithExternal(
	ctx context.Context, admin *models.SessionAdmin,
) (ReadSessionBinding, error) {
	return s.bindDelegatedRead(ctx, admin, true)
}

func (s *Service) bindDelegatedRead(
	ctx context.Context, admin *models.SessionAdmin, allowExternal bool,
) (ReadSessionBinding, error) {
	if s == nil || s.delegatedReads == nil || admin == nil || admin.ID <= 0 || admin.SubjectID.IsZero() ||
		admin.AuthSessionID == "" || admin.SessionID == "" {
		return ReadSessionBinding{}, ErrReadSessionEnded
	}
	if integrationroles.RolePreviewActive(ctx) {
		return ReadSessionBinding{}, ErrReadSessionDenied
	}
	seal, err := s.delegatedReads.seal()
	if err != nil {
		return ReadSessionBinding{}, err
	}
	//nolint:gosec // G407: NewGCMWithRandomNonce generates and prepends a fresh nonce.
	sealed := seal.Seal(nil, nil, []byte(admin.SessionID), []byte(admin.AuthSessionID))
	binding := ReadSessionBinding{
		storageKey: string(sealed), loginID: admin.AuthSessionID,
		subject: admin.SubjectID, adminID: admin.ID,
	}
	if !admin.ExternalValidUntil.IsZero() {
		if !allowExternal || s.states == nil {
			return ReadSessionBinding{}, ErrReadSessionEnded
		}
		record, found, loadErr := s.states.Load(ctx, admin.SessionID)
		if loadErr != nil {
			return ReadSessionBinding{}, loadErr
		}
		if !found || record.External == nil {
			return ReadSessionBinding{}, ErrReadSessionEnded
		}
		initial := *record.External
		binding.external = &initial
	}
	active, _, err := s.ResolveDelegatedRead(ctx, binding)
	if err != nil {
		return ReadSessionBinding{}, err
	}
	if integrationroles.RolePreviewActive(active) {
		return ReadSessionBinding{}, ErrReadSessionDenied
	}
	return binding, nil
}

// ResolveDelegatedRead never refreshes tokens, saves a Fiber session or
// extends an idle/absolute deadline. The caller still checks resource rights.
func (s *Service) ResolveDelegatedRead(ctx context.Context, b ReadSessionBinding) (context.Context, models.Admin, error) {
	sess, err := s.openDelegatedSession(ctx, b)
	if err != nil {
		return ctx, models.Admin{}, err
	}
	defer sess.Release()
	admin, err := s.delegatedAdmin(ctx, b)
	if err != nil {
		return ctx, models.Admin{}, err
	}
	if rawPreview := sess.Get(integrationroles.RolePreviewSessionKey); rawPreview != nil {
		id, ok := rawPreview.(int64)
		if !ok {
			return ctx, models.Admin{}, ErrReadSessionDenied
		}
		ctx, _, err = integrationroles.ResolveRolePreview(ctx, s.delegatedReads.isSuperAdmin(ctx, admin.ID), s.delegatedReads.roles, id)
		if errors.Is(err, integrationroles.ErrInvalidRolePreview) {
			return ctx, models.Admin{}, ErrReadSessionDenied
		}
		if err != nil {
			return ctx, models.Admin{}, err
		}
	}
	if err := s.checkDelegatedBrowser(ctx, b, sess); err != nil {
		return ctx, models.Admin{}, err
	}
	return ctx, admin, nil
}

func (s *Service) openDelegatedSession(ctx context.Context, b ReadSessionBinding) (*session.Session, error) {
	if s == nil || s.delegatedReads == nil || s.store == nil || s.states == nil || s.identityLinkSessions == nil ||
		s.admins == nil || b.storageKey == "" || b.adminID <= 0 || b.loginID == "" || b.subject.IsZero() {
		return nil, ErrReadSessionEnded
	}
	seal, err := s.delegatedReads.seal()
	if err != nil {
		return nil, err
	}
	raw, err := seal.Open(nil, nil, []byte(b.storageKey), []byte(b.loginID))
	if err != nil {
		return nil, ErrReadSessionEnded
	}
	// Keep Fiber's decoding and absolute deadline check. A delegated reader
	// must not delete a session while its own transaction holds the row.
	reader := session.Store{Config: s.store.Config}
	reader.Storage = delegatedSessionStorage{reader.Storage}
	sess, err := reader.GetByID(ctx, string(raw))
	if errors.Is(err, session.ErrSessionIDNotFoundInStore) {
		return nil, ErrReadSessionEnded
	}
	return sess, err
}

// GetByID calls DestroyWithContext on absolute expiry, using a background
// context. Let the native session lifecycle delete the stored row.
type delegatedSessionStorage struct{ fiber.Storage }

func (delegatedSessionStorage) DeleteWithContext(context.Context, string) error { return nil }
func (delegatedSessionStorage) Delete(string) error                             { return nil }

func (s *Service) delegatedAdmin(ctx context.Context, b ReadSessionBinding) (models.Admin, error) {
	security, err := s.identityLinkSessions.IntrospectSession(ctx, b.loginID)
	if errors.Is(err, goauth.ErrSessionRevoked) {
		return models.Admin{}, ErrReadSessionEnded
	}
	if err != nil {
		return models.Admin{}, err
	}
	login := security.Session
	if login.ID != b.loginID || login.SubjectID != b.subject || login.Realm != goauth.RealmAdmin ||
		login.Scope != goauth.SessionScopeAuthenticated || !login.Active(time.Now()) ||
		security.SubjectStatus != goauth.SubjectStatusActive || security.CurrentVersion != login.SecurityVersion {
		return models.Admin{}, ErrReadSessionEnded
	}
	admin, err := s.admins.GetBySubjectID(ctx, b.subject)
	if err != nil {
		return models.Admin{}, err
	}
	if admin.ID != b.adminID || admin.AuthSubjectID() != b.subject {
		return models.Admin{}, ErrReadSessionDenied
	}
	return admin, nil
}

func (s *Service) checkDelegatedBrowser(ctx context.Context, b ReadSessionBinding, sess *session.Session) error {
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil || state.SubjectID != b.subject || state.AuthSessionID != b.loginID {
		return ErrReadSessionEnded
	}
	record, found, err := s.states.Load(ctx, sess.ID())
	if err != nil {
		return err
	}
	if !found || record.SubjectID != b.subject || record.Tokens.Session.ID != b.loginID {
		return ErrReadSessionEnded
	}
	if record.Owner != "" {
		return ErrRefreshInProgress
	}
	hard, _ := sess.Get("goadmin.browserHardExpiresAt").(time.Time)
	expires := earliestMaintenanceExpiry(hard, state.HardExpiresAt, record.ExpiresAt, record.Tokens.Session.ExpiresAt)
	if expires.IsZero() || !time.Now().Before(expires) {
		return ErrReadSessionEnded
	}
	if b.external != nil {
		if state.External == nil || record.External == nil || record.External.LocalSubject != b.subject ||
			!sameExternalLogin(*b.external, *record.External) || !sameExternalLogin(*state.External, *record.External) {
			return ErrReadSessionEnded
		}
		return s.checkDelegatedExternal(ctx, *record.External)
	}
	if state.External != nil || record.External != nil {
		return ErrReadSessionEnded
	}
	if err := s.validateLocalLoginPublication(ctx, state); err != nil {
		if errors.Is(err, goauth.ErrSessionRevoked) {
			return ErrReadSessionEnded
		}
		return err
	}
	if s.localAdmission != nil {
		if err := s.localAdmission(ctx, b.subject); err != nil {
			if isTerminalSessionError(err) {
				return ErrReadSessionDenied
			}
			return err
		}
	}
	return nil
}

func (s *Service) checkDelegatedExternal(ctx context.Context, current ExternalBinding) error {
	link, err := s.canonicalExternalLink(ctx, current)
	if errors.Is(err, goauth.ErrExplicitIdentityLink) || errors.Is(err, goauth.ErrIdentityLinkNotFound) {
		return ErrReadSessionEnded
	}
	if err != nil {
		return err
	}
	if link.ID != current.LinkID {
		return ErrReadSessionEnded
	}
	if recoverableExternalDeadline(current, time.Now()) {
		return ErrExternalRecoveryRequired
	}
	_, err = s.externalProof(ctx, current)
	if errors.Is(err, goauth.ErrSessionRevoked) {
		return ErrReadSessionEnded
	}
	return err
}
