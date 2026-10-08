package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/adminbreadcrumbs"
	adminconfig "github.com/assurrussa/goadmin/config"
	"github.com/assurrussa/goadmin/infrastructure/core/inertia"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/services/adminservice"
	"github.com/assurrussa/goadmin/views"
)

// initInertia incapsulates the setup of Inertia.js bridge.
func initInertia(
	_ context.Context,
	cfg adminconfig.Config,
	deps Dependencies,
	menuExtra menu.Menu,
	opts ...goinertia.Option,
) (*goinertia.Inertia, error) {
	log := deps.System.Logger
	if log == nil {
		log = logger.Discard()
	}

	props := sharedProps(
		deps.Repos.FileLoader, deps.System.AdminNotificationRepo, deps.Repos.RolesService,
		menuExtra, log, cfg.BaseDomainURL,
	)
	if deps.Capabilities != nil {
		props["adminCapabilities"] = deps.Capabilities
	}
	optsInertia := []goinertia.Option{
		goinertia.WithFS(views.Templates),
		goinertia.WithLogger(log),
		goinertia.WithCustomErrorGettingHandler(inertia.CustomErrorGettingHandler),
		goinertia.WithSharedProps(props),
		goinertia.WithCSRFPropName(cfg.CSRFTokenName),
	}
	if cfg.IsLocal() {
		optsInertia = append(optsInertia,
			goinertia.WithCanExposeDetails(func(ctx context.Context, _ map[string][]string) bool {
				admin := adminmiddleware.GetAdminAuth(ctx)
				return admin != nil && admin.HasRole(integrationroles.SuperAdminRole)
			}),
			goinertia.WithDevMode(),
		)
	}

	sessStore := deps.System.SessionStore
	if sessStore != nil {
		optsInertia = append(optsInertia, goinertia.WithSessionStore(
			inertia.SessionBridge{Store: sessStore},
		))
	}

	optsInertia = append(optsInertia, goinertia.WithCSRFTokenCheckProvider(
		newCheckCSRFTokenProvider(sessStore, deps.System.CSRF, cfg), //nolint:contextcheck // provider uses request context from fiber
	))
	optsInertia = append(optsInertia, goinertia.WithCSRFTokenProvider(
		newCSRFTokenProvider( //nolint:contextcheck // provider uses request context from fiber
			deps.System.AdminAuthService,
			sessStore,
			deps.System.CSRF,
			cfg,
		),
	))

	optsInertia = append(optsInertia, opts...)

	return goinertia.NewWithValidation(cfg.DomainURL, optsInertia...)
}

func sharedProps(
	servicePreviewLoader FileLoader,
	notificationsRepo interface {
		CountUnread(ctx context.Context, adminID int64) (int, error)
	},
	menuChecker menu.PermissionChecker,
	menuExtra menu.Menu,
	log logger.Logger,
	publicDomainURL string,
) map[string]any {
	return map[string]any{
		"adminInfo": map[string]string{
			"version": adminVersion(),
		},
		"authuser": goinertia.LazyProp{
			Key: "authuser",
			Fn:  loadAuthUser(servicePreviewLoader, log),
		},
		"adminMenu": goinertia.LazyProp{
			Key: "adminMenu",
			Fn:  loadAdminMenu(menuChecker, menuExtra),
		},
		"adminBreadcrumbs": goinertia.LazyProp{
			Key: "adminBreadcrumbs",
			Fn:  loadAdminBreadcrumbs(menuChecker, menuExtra),
		},
		"notificationsSummary": goinertia.LazyProp{
			Key: "notificationsSummary",
			Fn:  loadNotificationsSummary(notificationsRepo, log),
		},
		"publicBaseURL": publicDomainURL,
	}
}

func loadAuthUser(servicePreviewLoader FileLoader, log logger.Logger) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		admin := adminmiddleware.GetAdminAuth(ctx)
		if admin == nil {
			return nil, nil
		}

		avatarURL := ""
		if admin.PreviewID > 0 && servicePreviewLoader != nil {
			file, err := servicePreviewLoader.LoadPreview(ctx, admin.PreviewID)
			if err != nil {
				log.ErrorContext(ctx, "Failed to load preview image", logger.Error(err))
			} else if file != nil {
				avatarURL = file.URL
			}
		}

		return map[string]any{
			"id":        admin.ID,
			"name":      admin.Name,
			"lastName":  admin.LastName,
			"email":     admin.Email,
			"roles":     admin.Roles,
			"avatarUrl": avatarURL,
		}, nil
	}
}

func loadAdminMenu(
	checker menu.PermissionChecker,
	extra menu.Menu,
) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		admin := adminmiddleware.GetAdminAuth(ctx)
		return menu.BuildAdminMenu(ctx, admin, checker, extra), nil
	}
}

func loadAdminBreadcrumbs(
	checker menu.PermissionChecker,
	extra menu.Menu,
) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		admin := adminmiddleware.GetAdminAuth(ctx)
		adminMenu := menu.BuildAdminMenu(ctx, admin, checker, extra)

		fiberCtx, ok := ctx.(fiber.Ctx)
		if !ok {
			return adminbreadcrumbs.BuildBreadcrumbs(adminMenu, "/"), nil
		}

		return adminbreadcrumbs.BuildBreadcrumbs(adminMenu, fiberCtx.Path()), nil
	}
}

func loadNotificationsSummary(repo interface {
	CountUnread(ctx context.Context, adminID int64) (int, error)
}, log logger.Logger,
) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		if repo == nil {
			return map[string]any{
				"unread":    0,     //nolint:goconst // required
				"hasUnread": false, //nolint:goconst // required
			}, nil
		}

		admin := adminmiddleware.GetAdminAuth(ctx)
		if admin == nil {
			return map[string]any{
				"unread":    0,
				"hasUnread": false,
			}, nil
		}

		unread, err := repo.CountUnread(ctx, admin.ID)
		if err != nil {
			log.ErrorContext(ctx, "failed to count admin notifications", logger.Error(err))
			unread = 0
		}

		return map[string]any{
			"unread":    unread,
			"hasUnread": unread > 0,
		}, nil
	}
}

func newCSRFTokenProvider(
	adminAuthService *adminservice.Service,
	store *session.Store,
	service *csrfservice.Service,
	cfg adminconfig.Config,
) goinertia.CSRFTokenProvider {
	if store == nil || service == nil || adminAuthService == nil {
		return nil
	}

	return func(c fiber.Ctx) (string, error) {
		if err := browsercookie.Check(c, store, getCookieTokenName(cfg)); err != nil {
			return "", goinertia.NewError(419, "Invalid browser credential", err)
		}
		sess, err := store.Get(c)
		if err != nil {
			return "", goinertia.NewError(fiber.StatusBadRequest, "failed admin session", err)
		}
		defer sess.Release()

		sessionID, err := adminAuthService.RegenerateSessionID(c, sess)
		if err != nil {
			return "", goinertia.NewError(fiber.StatusBadRequest, "failed getting session id", err)
		}

		sessionID = CSRFSessionBinding(sessionID)
		csrfTokenName := getCookieTokenName(cfg)
		adminID := adminmiddleware.GetAdminAuthUUID(c)

		if token := c.Cookies(csrfTokenName); token != "" {
			_, valid, err := service.Check(c, csrfservice.Request{
				SessionID: sessionID,
				UserID:    adminID, Domain: cfg.Domain,
			}, token)
			if err == nil && valid {
				return token, nil
			}
		}

		csrfToken, err := service.Create(c, csrfservice.Request{
			SessionID: sessionID,
			UserID:    adminID,
			Domain:    cfg.Domain,
		})
		if err != nil {
			return "", goinertia.NewError(419, "failed create CSRF token", err)
		}

		writeAdminCSRFCookie(c, store, cfg, csrfToken.Token, csrfToken.Domain)

		if adminID.IsZero() {
			sess.Set(string(coresession.GuestAdminKey), int8(1))
		} else {
			sess.Delete(string(coresession.GuestAdminKey))
		}

		if err := sess.Save(); err != nil {
			return "", goinertia.NewError(fiber.StatusBadRequest, "failed save cookie", err)
		}

		return csrfToken.Token, nil
	}
}

func newCheckCSRFTokenProvider(
	store *session.Store,
	service *csrfservice.Service,
	cfg adminconfig.Config,
) goinertia.CSRFTokenCheckProvider {
	if store == nil || service == nil {
		return nil
	}

	return func(c fiber.Ctx) error {
		if !validAdminOrigin(c, cfg) {
			return goinertia.NewError(419, "Invalid request origin")
		}
		if err := browsercookie.Check(c, store, getCookieTokenName(cfg)); err != nil {
			return goinertia.NewError(419, "Invalid browser credential", err)
		}
		sess, err := store.Get(c)
		if err != nil {
			return goinertia.NewError(fiber.StatusBadRequest, "failed admin session", err)
		}
		defer sess.Release()

		sessionID := sess.ID()
		if sessionID == "" {
			return goinertia.NewError(fiber.StatusBadRequest, "failed admin session id", err)
		}

		sessionID = CSRFSessionBinding(sessionID)
		adminID := adminmiddleware.GetAdminAuthUUID(c)

		csrfTokenName := getCookieTokenName(cfg)
		csrfTokenHeader := c.Get(csrfservice.HeaderName)
		csrfTokenCookie := c.Cookies(csrfTokenName)
		if csrfTokenHeader == "" || csrfTokenCookie == "" {
			return goinertia.NewError(419, "CSRF token is missing")
		}

		if csrfTokenHeader != csrfTokenCookie {
			return goinertia.NewError(419, "Invalid CSRF token")
		}

		_, valid, err := service.Check(c, csrfservice.Request{
			SessionID: sessionID, UserID: adminID, Domain: cfg.Domain,
		}, csrfTokenCookie)
		if err == nil && valid {
			return nil
		}

		return goinertia.NewError(419, "Invalid CSRF token")
	}
}

func getCookieTokenName(cfg adminconfig.Config) string {
	csrfTokenName := cfg.CSRFTokenName
	if csrfTokenName == "" {
		csrfTokenName = goinertia.ContextPropsCSRFToken
	}

	return csrfTokenName
}

// A readable signed proof must never expose the opaque authentication cookie.
// CSRFSessionBinding derives the native signed proof binding without exposing
// the bearer cookie in a readable JWT. Hosts must reject empty cookies first.
func CSRFSessionBinding(id string) string {
	sum := sha256.Sum256([]byte("goadmin-csrf-v1\x00" + id))
	return hex.EncodeToString(sum[:])
}

func validAdminOrigin(c fiber.Ctx, cfg adminconfig.Config) bool {
	expected, err := url.Parse(cfg.DomainURL)
	if err != nil || expected.User != nil || expected.Host == "" || (expected.Scheme != "https" && expected.Scheme != "http") {
		return false
	}
	origins := c.Request().Header.PeekAll(fiber.HeaderOrigin)
	if len(origins) > 1 {
		return false
	}
	raw := c.Get(fiber.HeaderOrigin)
	originPresent := len(origins) > 0
	if !originPresent {
		raw = c.Get(fiber.HeaderReferer)
	}
	actual, err := url.Parse(raw)
	if err != nil || actual.User != nil || actual.Scheme != expected.Scheme || actual.Host != expected.Host || actual.Host == "" {
		return false
	}
	if originPresent && (actual.Path != "" || actual.RawQuery != "" || actual.Fragment != "") {
		return false
	}
	return true
}

func writeAdminCSRFCookie(c fiber.Ctx, store *session.Store, cfg adminconfig.Config, token, domain string) {
	cookiePath := "/"
	cookieDomain := domain
	cookieSecure := cfg.IsProduction()
	cookieSameSite := fiber.CookieSameSiteLaxMode
	if store != nil {
		if store.CookiePath != "" {
			cookiePath = store.CookiePath
		}
		if store.CookieDomain != "" {
			cookieDomain = store.CookieDomain
		}
		cookieSecure = store.CookieSecure
		if store.CookieSameSite != "" {
			cookieSameSite = store.CookieSameSite
		}
	}

	c.Cookie(&fiber.Cookie{
		Name:     getCookieTokenName(cfg),
		Value:    token,
		HTTPOnly: false,
		Path:     cookiePath,
		Domain:   cookieDomain,
		SameSite: cookieSameSite,
		Secure:   cookieSecure,
		MaxAge:   int(cfg.CSRFTokenTTL.Seconds()),
	})
}
