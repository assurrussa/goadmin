package adminservice

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/realtimesession"
	"github.com/assurrussa/goadmin/models"
)

const (
	defaultSessionCookieName       = "session_id"
	sessionPersistenceCookieSuffix = "_persistent"
)

//nolint:gochecknoinits // Fiber's server-side session serializer uses gob.
func init() {
	gob.Register(&browserSession{})
	gob.Register(&models.SessionAdmin{})
}

type roleService interface {
	GetRolesAdmin(ctx context.Context, adminID int64) ([]integrationroles.Role, error)
	GetPermissions(
		ctx context.Context,
		roles []integrationroles.Role,
	) (map[int64]map[int64]integrationroles.Permission, error)
}

type runtime interface {
	Login(ctx context.Context, request goauth.LoginRequest) (goauth.LoginResult, error)
	ProvisionTrustedLocalAccount(ctx context.Context, request goauth.RegisterRequest) (goauth.Account, error)
	VerifyCredential(ctx context.Context, credential goauth.Credential) (goauth.Account, error)
	Refresh(ctx context.Context, refreshToken string) (goauth.TokenPair, error)
	VerifyAccessToken(ctx context.Context, token string, introspect bool) (goauth.AuthContext, error)
	GetAccount(ctx context.Context, subjectID goauth.SubjectID) (goauth.Account, error)
	Logout(ctx context.Context, subjectID goauth.SubjectID, sessionID string) error
	LogoutAll(ctx context.Context, subjectID goauth.SubjectID) (int64, error)
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, password string) error
	ChangePassword(ctx context.Context, request goauth.ChangePasswordRequest) (goauth.Account, error)
	RequestEmailChange(ctx context.Context, subjectID goauth.SubjectID, email string) error
	RequestEmailChangeWithPassword(ctx context.Context, request goauth.PasswordEmailChangeRequest) error
	PendingEmailChange(ctx context.Context, subjectID goauth.SubjectID) (goauth.PendingEmailChange, error)
	ConfirmEmailChange(ctx context.Context, subjectID goauth.SubjectID, code string) (goauth.Account, error)
	UpdateBasicProfile(
		ctx context.Context,
		subjectID goauth.SubjectID,
		profile goauth.BasicProfile,
	) (goauth.Account, error)
	SetSubjectStatus(
		ctx context.Context,
		subjectID goauth.SubjectID,
		status goauth.SubjectStatus,
	) (goauth.Subject, error)
}

type adminRepository interface {
	GetBySubjectID(ctx context.Context, subjectID goauth.SubjectID) (models.Admin, error)
}

type browserSession struct {
	External      *ExternalBinding
	Version       int64
	AccessToken   string
	RefreshToken  string
	AuthSessionID string
	SubjectID     goauth.SubjectID
	Payload       models.SessionAdmin
}

type Option func(*Service) error

type Service struct {
	externalAuthority ExternalSessionAuthority
	localAdmission    LocalAdminAdmission
	externalLinks     IdentityLinkResolver
	store             *session.Store
	roleService       roleService
	runtime           runtime
	admins            adminRepository
	sessionTTL        time.Duration
	states            browserstate.Store
	realtimeSessions  realtimesession.Registry
}

func NewService(store *session.Store, roles roleService, options ...Option) *Service {
	service := &Service{store: store, roleService: roles, sessionTTL: deriveSessionTTL(store)}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(service); err != nil {
			panic(fmt.Errorf("init admin auth service: %w", err))
		}
	}

	return service
}

func WithRuntime(authRuntime runtime, admins adminRepository) Option {
	return func(service *Service) error {
		if authRuntime == nil {
			return errors.New("goauth Runtime is required")
		}
		if admins == nil {
			return errors.New("admin membership repository is required")
		}
		service.runtime = authRuntime
		service.admins = admins

		return nil
	}
}

func (s *Service) LoginAdmin(
	c fiber.Ctx,
	email string,
	password string,
	persistent bool,
) (models.Admin, goauth.Account, error) {
	if s == nil || s.runtime == nil || s.store == nil {
		return models.Admin{}, goauth.Account{}, errors.New("admin auth Runtime is not configured")
	}
	result, err := s.runtime.Login(c, goauth.LoginRequest{
		Credential: goauth.Credential{
			Identifier: goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: email},
			Password:   password,
		},
		Realm: goauth.RealmAdmin,
	})
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	if s.localAdmission != nil {
		if err := s.localAdmission(c, result.Account.Subject.ID); err != nil {
			_ = s.runtime.Logout(context.WithoutCancel(c.Context()), result.Account.Subject.ID, result.Tokens.Session.ID)
			return models.Admin{}, goauth.Account{}, err
		}
	}
	admin, err := s.admins.GetBySubjectID(c, result.Account.Subject.ID)
	if err != nil || admin.ID <= 0 {
		_ = s.runtime.Logout(c, result.Account.Subject.ID, result.Tokens.Session.ID)
		if err == nil {
			err = goauth.ErrMembershipDenied
		}
		return models.Admin{}, goauth.Account{}, fmt.Errorf("load admin membership: %w", err)
	}
	if err := s.saveAuthenticatedSession(c, admin, result.Account, result.Tokens, persistent); err != nil {
		_ = s.runtime.Logout(c, result.Account.Subject.ID, result.Tokens.Session.ID)
		return models.Admin{}, goauth.Account{}, err
	}

	return admin, result.Account, nil
}

func (s *Service) ProvisionTrustedAdmin(
	ctx context.Context,
	request goauth.RegisterRequest,
) (goauth.Account, error) {
	if s == nil || s.runtime == nil {
		return goauth.Account{}, errors.New("admin auth Runtime is not configured")
	}
	if account, found, err := s.provisionOrFind(ctx, request); err != nil || !found {
		return account, err
	}
	account, verifyErr := s.runtime.VerifyCredential(ctx, goauth.Credential{
		Identifier: goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: request.Email},
		Password:   request.Password,
	})
	if verifyErr != nil {
		return goauth.Account{}, fmt.Errorf("verify existing account credentials: %w", verifyErr)
	}
	if !account.EmailVerified() {
		return goauth.Account{}, goauth.ErrInvalidCredentials
	}

	return account, nil
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	return s.runtime.RequestPasswordReset(ctx, email)
}

func (s *Service) PerformPasswordReset(ctx context.Context, token, password string) error {
	return s.runtime.ResetPassword(ctx, token, password)
}

func (s *Service) ChangePassword(
	ctx context.Context,
	subjectID goauth.SubjectID,
	currentPassword string,
	newPassword string,
) error {
	_, err := s.runtime.ChangePassword(ctx, goauth.ChangePasswordRequest{
		SubjectID: subjectID, CurrentPassword: currentPassword, NewPassword: newPassword,
	})

	return err
}

func (s *Service) RequestEmailChange(
	ctx context.Context,
	subjectID goauth.SubjectID,
	newEmail string,
) error {
	return s.runtime.RequestEmailChange(ctx, subjectID, newEmail)
}

func (s *Service) RequestEmailChangeWithPassword(
	ctx context.Context,
	subjectID goauth.SubjectID,
	currentPassword string,
	newEmail string,
) error {
	return s.runtime.RequestEmailChangeWithPassword(ctx, goauth.PasswordEmailChangeRequest{
		SubjectID: subjectID, CurrentPassword: currentPassword, NewEmail: newEmail,
	})
}

func (s *Service) ConfirmEmailChange(
	ctx context.Context,
	subjectID goauth.SubjectID,
	code string,
) (string, error) {
	account, err := s.runtime.ConfirmEmailChange(ctx, subjectID, code)
	if err != nil {
		return "", err
	}

	return account.PrimaryEmail.DisplayValue, nil
}

func (s *Service) GetPendingEmailChange(
	ctx context.Context,
	subjectID goauth.SubjectID,
) (*goauth.PendingEmailChange, error) {
	pending, err := s.runtime.PendingEmailChange(ctx, subjectID)
	if err != nil {
		return nil, err
	}

	return &pending, nil
}

func (s *Service) UpdateBasicProfile(
	ctx context.Context,
	subjectID goauth.SubjectID,
	profile goauth.BasicProfile,
) (goauth.Account, error) {
	return s.runtime.UpdateBasicProfile(ctx, subjectID, profile)
}

func (s *Service) SetSubjectStatus(
	ctx context.Context,
	subjectID goauth.SubjectID,
	status goauth.SubjectStatus,
) (goauth.Subject, error) {
	return s.runtime.SetSubjectStatus(ctx, subjectID, status)
}

func (s *Service) GetAdminAuth(c fiber.Ctx) (*models.SessionAdmin, error) {
	if s == nil || s.store == nil || s.runtime == nil {
		return nil, errors.New("admin auth Runtime is not configured")
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return nil, err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return nil, fmt.Errorf("get admin browser session: %w", err)
	}
	defer sess.Release()

	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil || (s.states == nil && (state.AccessToken == "" || state.RefreshToken == "")) {
		return nil, nil //nolint:nilnil // no authenticated browser session
	}
	active, err := s.loadAuthenticatedAuthority(c, sess, state)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, nil //nolint:nilnil // requires a new login
	}

	authContext, err := s.runtime.VerifyAccessToken(c, state.AccessToken, true)
	if err != nil {
		refreshed, refreshErr := s.refreshAdminToken(c, sess, state, err)
		if refreshErr != nil {
			return nil, refreshErr
		}
		if refreshed == nil {
			return nil, nil //nolint:nilnil // expired or revoked auth is anonymous
		}
		authContext = *refreshed
	}
	if authContext.Realm != goauth.RealmAdmin || authContext.SubjectID != state.SubjectID {
		return nil, goauth.ErrInvalidToken
	}
	account, err := s.runtime.GetAccount(c, state.SubjectID)
	if err != nil {
		if isTerminalSessionError(err) {
			if clearErr := s.clearAdminBrowserSession(c, sess); clearErr != nil {
				return nil, clearErr
			}
			return nil, nil //nolint:nilnil // unavailable canonical account is anonymous
		}
		return nil, fmt.Errorf("load canonical admin account: %w", err)
	}
	admin, err := s.admins.GetBySubjectID(c, state.SubjectID)
	if err != nil {
		return nil, fmt.Errorf("load admin membership: %w", err)
	}
	if admin.ID <= 0 {
		if err := s.clearAdminBrowserSession(c, sess); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // removed admin membership becomes an anonymous browser session
	}
	state.Payload, err = s.buildSessionAdmin(c, admin, sess.ID())
	if err != nil {
		return nil, err
	}
	state.Payload.Email = account.PrimaryEmail.DisplayValue
	if s.states != nil {
		state.AccessToken = ""
		state.RefreshToken = ""
	}
	sess.Set(sessioncore.AuthAdminKey.String(), state)
	if err := s.saveAuthenticatedProjection(c, sess, state); err != nil {
		return nil, fmt.Errorf("save refreshed admin browser session: %w", err)
	}

	payload := cloneSessionAdmin(state.Payload)
	// Only canonical, introspected identity reaches the WebSocket lifecycle key.
	// Never reuse these values from a persisted browser projection.
	payload.SubjectID = authContext.SubjectID
	payload.AuthSessionID = authContext.SessionID
	return &payload, nil
}

func (s *Service) refreshAdminToken(
	c fiber.Ctx,
	sess *session.Session,
	state *browserSession,
	verifyErr error,
) (*goauth.AuthContext, error) {
	if isTerminalSessionError(verifyErr) {
		if err := s.clearAdminBrowserSession(c, sess); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // revoked account or session is anonymous
	}
	if !errors.Is(verifyErr, goauth.ErrExpiredToken) && !errors.Is(verifyErr, goauth.ErrInvalidToken) {
		return nil, fmt.Errorf("introspect admin token: %w", verifyErr)
	}
	if s.states != nil {
		return s.refreshOwnedAdminToken(c, sess, state)
	}
	rotated, err := s.runtime.Refresh(c, state.RefreshToken)
	if err != nil {
		if !isInvalidRefreshToken(err) {
			return nil, fmt.Errorf("refresh admin token: %w", err)
		}
		if err := s.clearAdminBrowserSession(c, sess); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // expired or revoked auth is anonymous
	}
	state.AccessToken = rotated.AccessToken
	state.RefreshToken = rotated.RefreshToken
	state.AuthSessionID = rotated.Session.ID
	authContext, err := s.runtime.VerifyAccessToken(c, state.AccessToken, true)
	if err != nil {
		if isTerminalSessionError(err) {
			if clearErr := s.clearAdminBrowserSession(c, sess); clearErr != nil {
				return nil, clearErr
			}
			return nil, nil //nolint:nilnil // refreshed session was revoked concurrently
		}
		return nil, fmt.Errorf("introspect refreshed admin token: %w", err)
	}
	return &authContext, nil
}

func (s *Service) clearAdminBrowserSession(c fiber.Ctx, sess *session.Session) error {
	if s.states != nil {
		if err := s.states.Delete(c, sess.ID()); err != nil {
			return fmt.Errorf("delete admin auth state: %w", err)
		}
	}
	sess.Delete(sessioncore.AuthAdminKey.String())
	if err := sess.Save(); err != nil {
		return fmt.Errorf("delete invalid admin browser session: %w", err)
	}
	s.deleteSessionCookies(c)
	return nil
}

func (s *Service) saveAuthenticatedSession(
	c fiber.Ctx,
	admin models.Admin,
	account goauth.Account,
	tokens goauth.TokenPair,
	persistent bool,
	external ...*ExternalBinding,
) error {
	if err := browsercookie.Check(c, s.store); err != nil {
		return err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return fmt.Errorf("get admin browser session: %w", err)
	}
	defer sess.Release()
	if err := sess.Regenerate(); err != nil {
		return fmt.Errorf("rotate admin browser session: %w", err)
	}
	payload, err := s.buildSessionAdmin(c, admin, sess.ID())
	if err != nil {
		return err
	}
	payload.Email = account.PrimaryEmail.DisplayValue
	state := &browserSession{
		Version:     1,
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		AuthSessionID: tokens.Session.ID, SubjectID: account.Subject.ID, Payload: payload,
	}
	if len(external) > 0 && external[0] != nil {
		binding := *external[0]
		state.External = &binding
		state.Payload.ExternalValidUntil = binding.Deadline
	}
	if s.states != nil {
		state.AccessToken = ""
		state.RefreshToken = ""
	}
	sess.Set(sessioncore.AuthAdminKey.String(), state)
	if s.states != nil {
		ttl := min(s.sessionTTL, time.Until(tokens.Session.ExpiresAt))
		if err := s.states.Create(c, sess.ID(), browserstate.Record{SubjectID: account.Subject.ID, Tokens: tokens}, ttl); err != nil {
			return fmt.Errorf("create admin auth state: %w", err)
		}
	}
	if err := sess.Save(); err != nil {
		if s.states != nil {
			_ = s.states.Delete(c, sess.ID())
		}
		return fmt.Errorf("save admin browser session: %w", err)
	}
	s.writeSessionCookie(c, sess.ID(), persistent)

	return nil
}

// DelAdminAuth is explicit logout, including termination of an attached RP
// session even if its saved generation has been superseded by renewal.
func (s *Service) DelAdminAuth(c fiber.Ctx) error {
	return s.delAdminAuth(c, true)
}

func (s *Service) delAdminAuth(c fiber.Ctx, userLogout bool) (result error) {
	if s == nil || s.store == nil {
		return nil
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return fmt.Errorf("get admin browser session: %w", err)
	}
	defer sess.Release()
	var external *ExternalBinding
	original, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if ok && original != nil && original.External != nil {
		binding := *original.External
		external = &binding
	}
	defer func() { result = errors.Join(result, s.finishExternal(c.Context(), external, userLogout)) }()
	var revokeErr error
	if s.states != nil {
		record, found, loadErr := s.states.Load(c, sess.ID())
		if loadErr != nil {
			return fmt.Errorf("load logout admin auth state: %w", loadErr)
		}
		if found {
			sess.Set(sessioncore.AuthAdminKey.String(), &browserSession{
				External:      external,
				SubjectID:     record.SubjectID,
				AuthSessionID: record.Tokens.Session.ID,
			})
		}
		if err := s.states.Delete(c, sess.ID()); err != nil {
			return fmt.Errorf("delete logout admin auth state: %w", err)
		}
	}
	if state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession); ok && state != nil && s.runtime != nil {
		if err := s.runtime.Logout(c, state.SubjectID, state.AuthSessionID); err != nil && !errors.Is(err, goauth.ErrSessionRevoked) {
			revokeErr = fmt.Errorf("revoke admin Runtime session: %w", err)
		} else if userLogout {
			// Canonical revocation has succeeded even if browser persistence or
			// deferred external cleanup fails. Detach and global revocation keep
			// their existing policy; this hook is explicit session logout only.
			s.realtimeSessions.Revoke(realtimesession.Key{SubjectID: state.SubjectID, AuthSessionID: state.AuthSessionID})
		}
	}
	sess.Delete(sessioncore.AuthAdminKey.String())
	var deleteErr error
	if err := sess.Save(); err != nil {
		deleteErr = fmt.Errorf("delete admin browser session: %w", err)
	}
	s.deleteSessionCookies(c)

	return errors.Join(revokeErr, deleteErr)
}

func (s *Service) ApplyCookiePolicy(c fiber.Ctx, persistent bool) error {
	if s == nil || s.store == nil {
		return nil
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return fmt.Errorf("get admin browser session: %w", err)
	}
	defer sess.Release()
	if sess.ID() == "" {
		return errors.New("admin browser session id is required")
	}
	s.writeSessionCookie(c, sess.ID(), persistent)

	return nil
}

func (s *Service) DeleteAll(ctx context.Context, subjectID goauth.SubjectID) (int64, error) {
	if s == nil || s.runtime == nil || subjectID.IsZero() {
		return 0, nil
	}

	return s.runtime.LogoutAll(ctx, subjectID)
}

// DeleteAllExcept revokes every canonical session, including the supplied one.
// The keep token remains in the signature for compatibility with existing callers;
// goauth security-version changes cannot preserve a session issued before them.
func (s *Service) DeleteAllExcept(
	ctx context.Context,
	subjectID goauth.SubjectID,
	_ string,
) (int64, error) {
	// Password/status changes bump security_version, so retaining a pre-change
	// session would defeat the v0.2 revocation invariant.
	return s.DeleteAll(ctx, subjectID)
}

func isInvalidRefreshToken(err error) bool {
	return errors.Is(err, goauth.ErrInvalidToken) || errors.Is(err, goauth.ErrExpiredToken) ||
		errors.Is(err, goauth.ErrSessionRevoked) || errors.Is(err, goauth.ErrRefreshReplay) ||
		errors.Is(err, goauth.ErrSecurityVersionMismatch) ||
		errors.Is(err, goauth.ErrAccountUnavailable) || errors.Is(err, goauth.ErrAccountNotFound)
}

func isTerminalSessionError(err error) bool {
	return errors.Is(err, goauth.ErrMembershipDenied) || errors.Is(err, goauth.ErrSessionRevoked) ||
		errors.Is(err, goauth.ErrSecurityVersionMismatch) ||
		errors.Is(err, goauth.ErrAccountUnavailable) || errors.Is(err, goauth.ErrAccountNotFound)
}

func (s *Service) RotateAdminAuth(c fiber.Ctx, _ models.Admin) (string, error) {
	if s == nil || s.store == nil {
		return "", errors.New("admin browser session store is required")
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return "", err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil {
		return "", goauth.ErrSessionRevoked
	}
	if s.states != nil {
		if err := s.rotateOwnedBrowserID(c, sess, state); err != nil {
			return "", err
		}
	} else if err := sess.Regenerate(); err != nil {
		return "", err
	}
	state.Payload.SessionID = sess.ID()
	sess.Set(sessioncore.AuthAdminKey.String(), state)
	if err := sess.Save(); err != nil {
		return "", err
	}

	return sess.ID(), nil
}

func (s *Service) RegenerateSessionID(_ context.Context, sess *session.Session) (string, error) {
	if sess == nil {
		return "", errors.New("session is nil")
	}
	if sess.ID() == "" {
		if err := sess.Regenerate(); err != nil {
			return "", fmt.Errorf("regenerate browser session: %w", err)
		}
	}
	if state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession); ok && state != nil {
		state.Payload.SessionID = sess.ID()
		sess.Set(sessioncore.AuthAdminKey.String(), state)
		if err := sess.Save(); err != nil {
			return "", err
		}
	}

	return sess.ID(), nil
}

func (s *Service) UpdatePreview(ctx context.Context, sessionID string, previewID int64) error {
	if s == nil || s.store == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	sess, err := s.store.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get admin browser session by id: %w", err)
	}
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil {
		return errors.New("admin browser session payload missing")
	}
	state.Payload.PreviewID = previewID
	sess.Set(sessioncore.AuthAdminKey.String(), state)

	return sess.Save()
}

func (s *Service) buildSessionAdmin(ctx context.Context, admin models.Admin, sessionID string) (models.SessionAdmin, error) {
	subjectID := admin.AuthSubjectID()
	if subjectID.IsZero() {
		return models.SessionAdmin{}, errors.New("admin subject id is required")
	}
	roles, err := s.roleService.GetRolesAdmin(ctx, admin.ID)
	if err != nil {
		return models.SessionAdmin{}, fmt.Errorf("get admin roles: %w", err)
	}
	roleNames := make([]string, 0, len(roles))
	roleSlugs := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
		roleSlugs = append(roleSlugs, role.Slug)
	}

	return models.SessionAdmin{
		ID: admin.ID, SubjectID: subjectID, UUID: admin.UUID, Username: admin.Username,
		Name: admin.Name, LastName: admin.LastName, PreviewID: admin.GetPreviewFileID(),
		Email: admin.Email, Roles: roleNames, RoleSlugs: roleSlugs,
		Permissions: maps.Clone(admin.GetPermissions()), SessionID: sessionID,
	}, nil
}

func cloneSessionAdmin(admin models.SessionAdmin) models.SessionAdmin {
	clone := admin
	clone.Roles = sliceClone(admin.Roles)
	clone.RoleSlugs = sliceClone(admin.RoleSlugs)
	clone.Permissions = maps.Clone(admin.Permissions)
	for key, values := range clone.Permissions {
		clone.Permissions[key] = sliceClone(values)
	}

	return clone
}

func sliceClone[T any](values []T) []T { return append([]T(nil), values...) }

func deriveSessionTTL(store *session.Store) time.Duration {
	const fallback = 24 * time.Hour
	if store == nil {
		return fallback
	}
	if store.AbsoluteTimeout > 0 {
		return store.AbsoluteTimeout
	}
	if store.IdleTimeout > 0 {
		return store.IdleTimeout
	}

	return fallback
}

func (s *Service) writeSessionCookie(c fiber.Ctx, sessionID string, persistent bool) {
	if s.store == nil || sessionID == "" {
		return
	}
	sessionCookie := fiber.Cookie{
		Name: s.sessionCookieName(), Value: sessionID, Path: s.store.CookiePath,
		Domain: s.store.CookieDomain, Secure: s.store.CookieSecure,
		HTTPOnly: s.store.CookieHTTPOnly, SameSite: s.store.CookieSameSite,
	}
	policyCookie := fiber.Cookie{
		Name: s.policyCookieName(), Value: "0", Path: s.store.CookiePath,
		Domain: s.store.CookieDomain, Secure: s.store.CookieSecure,
		HTTPOnly: true, SameSite: s.store.CookieSameSite,
	}
	if persistent {
		expiresAt := time.Now().Add(s.sessionTTL)
		maxAge := int(s.sessionTTL.Seconds())
		sessionCookie.MaxAge, sessionCookie.Expires = maxAge, expiresAt
		policyCookie.MaxAge, policyCookie.Expires, policyCookie.Value = maxAge, expiresAt, "1"
	}
	c.Cookie(&sessionCookie)
	c.Cookie(&policyCookie)
}

func (s *Service) deletePolicyCookie(c fiber.Ctx) {
	if s.store == nil {
		return
	}
	c.Cookie(&fiber.Cookie{
		Name: s.policyCookieName(), Path: s.store.CookiePath, Domain: s.store.CookieDomain,
		Secure: s.store.CookieSecure, HTTPOnly: true, SameSite: s.store.CookieSameSite,
		MaxAge: -1, Expires: time.Unix(0, 0),
	})
}

func (s *Service) deleteSessionCookies(c fiber.Ctx) {
	if s.store == nil {
		return
	}
	c.Cookie(&fiber.Cookie{
		Name: s.sessionCookieName(), Path: s.store.CookiePath, Domain: s.store.CookieDomain,
		Secure: s.store.CookieSecure, HTTPOnly: s.store.CookieHTTPOnly, SameSite: s.store.CookieSameSite,
		MaxAge: -1, Expires: time.Unix(0, 0),
	})
	s.deletePolicyCookie(c)
}

func (s *Service) sessionCookieName() string {
	if s.store == nil || s.store.Extractor.Key == "" {
		return defaultSessionCookieName
	}

	return s.store.Extractor.Key
}

func (s *Service) policyCookieName() string {
	return s.sessionCookieName() + sessionPersistenceCookieSuffix
}

func (s *Service) loadBrowserAuthority(c fiber.Ctx, sess *session.Session, state *browserSession) (bool, error) {
	if s.states == nil {
		return true, nil
	}
	record, found, loadErr := s.states.Load(c, sess.ID())
	if loadErr != nil {
		return false, fmt.Errorf("load admin auth state: %w", loadErr)
	}
	if !found {
		s.discardStaleSessionCookies(c)
		return false, nil
	}
	applyBrowserRecord(state, record)
	return true, nil
}

func (s *Service) provisionOrFind(ctx context.Context, request goauth.RegisterRequest) (goauth.Account, bool, error) {
	finder, ok := s.runtime.(interface {
		FindAccount(ctx context.Context, input goauth.IdentifierInput) (goauth.Account, error)
	})
	if !ok {
		account, err := s.runtime.ProvisionTrustedLocalAccount(ctx, request)
		if errors.Is(err, goauth.ErrIdentifierAlreadyExists) {
			return goauth.Account{}, true, nil
		}
		return account, false, err
	}
	account, err := finder.FindAccount(ctx, goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: request.Email})
	if errors.Is(err, goauth.ErrAccountNotFound) {
		created, createErr := s.runtime.ProvisionTrustedLocalAccount(ctx, request)
		return created, false, createErr
	}
	return account, true, err
}

// PrepareTrustedAdmin separates attempt admission from the atomic host command.
func (s *Service) PrepareTrustedAdmin(ctx context.Context, request goauth.RegisterRequest) (
	func(context.Context) (goauth.Account, error), error,
) {
	if runtime, ok := s.runtime.(integrationroles.TrustedAccountRuntime); ok {
		return integrationroles.PrepareTrustedAccount(ctx, runtime, request)
	}
	return func(ctx context.Context) (goauth.Account, error) { return s.ProvisionTrustedAdmin(ctx, request) }, nil
}
