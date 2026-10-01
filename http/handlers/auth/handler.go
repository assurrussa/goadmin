package auth

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gobuffalo/validate/validators"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/infrastructure/core/formvalidator"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/authdelay"
	identity "github.com/assurrussa/goadmin/internal/identity"
	adminloginaudit "github.com/assurrussa/goadmin/loginaudit"
	models2 "github.com/assurrussa/goadmin/models"
	adminshared "github.com/assurrussa/goadmin/shared"
)

//go:generate toolsmocks

type adminRepo interface {
	ProvisionAccount(
		ctx context.Context,
		account goauth.Account,
		publicID identity.UserID,
	) (models2.Admin, error)
	GetByID(ctx context.Context, id int64) (models2.Admin, error)
	GetByUUID(ctx context.Context, id identity.UserID) (models2.Admin, error)
	UpdateAdmin(ctx context.Context, id int64, admin models2.Admin) error
}

type previewService interface {
	LoadPreview(ctx context.Context, fileID int64) (*uploadhost.File, error)
}

type sessionStoreActions interface {
	LoginAdmin(c fiber.Ctx, email, password string, persistent bool) (models2.Admin, goauth.Account, error)
	ProvisionTrustedAdmin(ctx context.Context, request goauth.RegisterRequest) (goauth.Account, error)
	ApplyCookiePolicy(c fiber.Ctx, persistent bool) error
	DelAdminAuth(c fiber.Ctx) error
	RotateAdminAuth(c fiber.Ctx, user models2.Admin) (string, error)
	DeleteAll(ctx context.Context, subjectID authcore.SubjectID) (int64, error)
	DeleteAllExcept(ctx context.Context, subjectID authcore.SubjectID, keepToken string) (int64, error)
}

type adminAccountService interface {
	RequestPasswordReset(ctx context.Context, email string) error
	PerformPasswordReset(ctx context.Context, token, password string) error
	ChangePassword(ctx context.Context, subjectID authcore.SubjectID, currentPassword, newPassword string) error
	RequestEmailChangeWithPassword(ctx context.Context, subjectID authcore.SubjectID, currentPassword, newEmail string) error
	ConfirmEmailChange(ctx context.Context, subjectID authcore.SubjectID, code string) (string, error)
	GetPendingEmailChange(ctx context.Context, subjectID authcore.SubjectID) (*goauth.PendingEmailChange, error)
	UpdateBasicProfile(
		ctx context.Context,
		subjectID goauth.SubjectID,
		profile goauth.BasicProfile,
	) (goauth.Account, error)
}

type adminLoginAuditWriter interface {
	RecordSuccessfulLogin(ctx context.Context, record adminloginaudit.Record) error
}

type firstAdminSetupTokens interface {
	Validate(ctx context.Context, token string) error
	Consume(ctx context.Context, token string) error
	StaticConfigured() bool
}

// Handler обрабатывает запросы, связанные с аутентификацией.
type Handler struct {
	adminApp            *adminapp.App
	sleep               time.Duration
	adminRepo           adminRepo
	rolesManager        rolesBootstrapManager
	previewService      previewService
	adminAccount        adminAccountService
	sessionStoreActions sessionStoreActions
	loginAudit          adminLoginAuditWriter
	setupTokens         firstAdminSetupTokens
	actionAudit         adminactionauditrepo.Writer
	baseURL             string
	adminMenu           menu.Menu
}

// NewHandler создает новый обработчик аутентификации.
type HandlerOptions struct {
	AdminApp            *adminapp.App
	Sleep               time.Duration
	AdminRepo           adminRepo
	RolesManager        rolesBootstrapManager
	PreviewService      previewService
	AdminAccountService adminAccountService
	SessionStoreActions sessionStoreActions
	AdminLoginAudit     adminLoginAuditWriter
	FirstAdminSetup     firstAdminSetupTokens
	ActionAudit         adminactionauditrepo.Writer
	AdminDomainURL      string
	AdminMenu           menu.Menu
}

func NewHandler(opts HandlerOptions) *Handler {
	accountService := opts.AdminAccountService
	if accountService == nil {
		accountService, _ = opts.SessionStoreActions.(adminAccountService)
	}

	return &Handler{
		adminApp:            opts.AdminApp,
		sleep:               opts.Sleep,
		adminRepo:           opts.AdminRepo,
		rolesManager:        opts.RolesManager,
		previewService:      opts.PreviewService,
		adminAccount:        accountService,
		sessionStoreActions: opts.SessionStoreActions,
		loginAudit:          opts.AdminLoginAudit,
		setupTokens:         opts.FirstAdminSetup,
		actionAudit:         opts.ActionAudit,
		baseURL:             opts.AdminDomainURL,
		adminMenu:           opts.AdminMenu,
	}
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	redirectAuthenticated := adminmiddleware.IsAuthAdminMiddleware(func(c fiber.Ctx, admin *models2.SessionAdmin) string {
		return h.buildPostLoginRedirect(c, admin.ID)
	})
	route.Get("/auth/login", redirectAuthenticated, h.GetLogin)
	route.Post("/auth/login", redirectAuthenticated, h.PostLogin)
	route.Get("/auth/register", redirectAuthenticated, h.GetRegister)
	route.Post("/auth/register", redirectAuthenticated, h.PostRegister)
	if h.adminApp.Enabled("authmail") {
		route.Get("/auth/forgot-password", h.ForgotPassword)
		route.Post("/auth/forgot-password", h.PostForgotPassword)
		route.Get("/reset-password", h.ResetPassword)
		route.Post("/reset-password", h.PostResetPassword)
		route.Get("/auth/reset-password", h.ResetPassword)
		route.Post("/auth/reset-password", h.PostResetPassword)
	}
	route.Delete("/auth/logout", h.Logout)
	route.Get("/auth/profile", h.Profile)
	route.Get("/auth/profile/edit", h.ProfileEdit)
	route.Post("/auth/profile/edit", h.PostProfileEdit)
	route.Get("/auth/profile/settings", h.ProfileSettings)
	route.Post("/auth/profile/settings/password", h.PostProfileChangePassword)
	if h.adminApp.Enabled("authmail") {
		route.Post("/auth/profile/settings/email/request", h.PostProfileEmailRequest)
		route.Post("/auth/profile/settings/email/confirm", h.PostProfileEmailConfirm)
	}
}

type FormLogin struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

type FormForgotPassword struct {
	Email string `json:"email"`
}

type FormResetPassword struct {
	Token           string `json:"token"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"passwordConfirmation"`
}

type FormProfileEdit struct {
	Name     string `json:"name"`
	LastName string `json:"lastName"`
	Username string `json:"username"`
}

type FormProfileChangePassword struct {
	CurrentPassword string `json:"currentPassword"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"passwordConfirmation"`
}

type FormProfileEmailRequest struct {
	CurrentPassword string `json:"currentPassword"`
	Email           string `json:"email"`
}

type FormProfileEmailConfirm struct {
	Code string `json:"code"`
}

func (h *Handler) GetLogin(c fiber.Ctx) error {
	canRegisterFirstAdmin := false
	if h.setupTokens != nil && h.setupTokens.StaticConfigured() {
		available, err := h.firstAdminRegistrationAvailable(c)
		if err != nil {
			h.adminApp.Logger().ErrorContext(c, "check first admin bootstrap", logger.Error(err))
		} else {
			canRegisterFirstAdmin = available
		}
	}

	return h.adminApp.HTTPManager().Render(c, "auth/LoginPage", map[string]any{
		"title":                 "Вход в админ панель", //nolint:goconst // required
		"canRegisterFirstAdmin": canRegisterFirstAdmin,
	})
}

func (h *Handler) PostLogin(c fiber.Ctx) error {
	var form FormLogin
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.EmailIsPresent{Name: "Email", Field: form.Email, Message: "Email не задан или неверный"},           //nolint:goconst,lll // required
		&validators.StringIsPresent{Name: "Password", Field: form.Password, Message: "Password не задан или неверный"}, //nolint:goconst,lll // required
	)
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if h.sessionStoreActions == nil {
		err := errors.New("admin auth Runtime is not configured")
		h.adminApp.Logger().ErrorContext(c, "admin auth Runtime unavailable", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13341")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	adminUser, account, err := h.sessionStoreActions.LoginAdmin(c, form.Email, form.Password, form.Remember)
	if err != nil {
		h.adminApp.Logger().WarnContext(c, "admin Runtime login rejected", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Неправильно введен пароль или email!")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	h.recordSuccessfulLogin(c, account.Subject.ID, adminUser)
	h.touchAdminLoginMetadata(c, adminUser.ID, time.Now().UTC())

	urlRedirect := h.buildPostLoginRedirect(c, adminUser.ID)
	return h.adminApp.HTTPManager().Redirect(c, urlRedirect)
}

// ForgotPassword обрабатывает запрос на восстановление пароля.
func (h *Handler) ForgotPassword(c fiber.Ctx) error {
	return h.adminApp.HTTPManager().Render(c, "auth/ForgotPasswordPage", map[string]any{
		"title": "Забыли пароль?",
	})
}

// PostForgotPassword обрабатывает запрос на восстановление пароля.
func (h *Handler) PostForgotPassword(c fiber.Ctx) error {
	var form FormForgotPassword
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.EmailIsPresent{Name: "Email", Field: form.Email, Message: "Email не задан или неверный"},
	)
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	if h.adminAccount == nil {
		err := errors.New("admin account service is not configured")
		h.adminApp.Logger().ErrorContext(c, "admin account service unavailable", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13344")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if err := h.adminAccount.RequestPasswordReset(c, form.Email); err != nil {
		h.adminApp.Logger().ErrorContext(c, "admin request password reset", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13344")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Письмо с кодом подтверждения отправлено!")

	return h.adminApp.HTTPManager().RedirectBack(c)
}

// ResetPassword обрабатывает сброс пароля.
func (h *Handler) ResetPassword(c fiber.Ctx) error {
	token := c.Query("token")

	return h.adminApp.HTTPManager().Render(c, "auth/ResetPasswordPage", map[string]any{
		"title": "Сброс пароля",
		"token": token,
	})
}

// PostResetPassword обрабатывает запрос на сброс пароля.
func (h *Handler) PostResetPassword(c fiber.Ctx) error {
	var form FormResetPassword
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.StringIsPresent{Name: "Token", Field: form.Token, Message: "Token не задан или неверный"},
		&validators.StringIsPresent{Name: "Password", Field: form.Password, Message: "Password не задан или неверный"},
		&validators.StringIsPresent{
			Name: "ConfirmPassword", Field: form.ConfirmPassword, Message: "ConfirmPassword не задан или неверный",
		},
	)
	if form.Password != form.ConfirmPassword {
		errs.Add("passwordConfirmation", "Пароли не совпадают")
	}
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if h.adminAccount == nil {
		err := errors.New("admin account service is not configured")
		h.adminApp.Logger().ErrorContext(c, "admin account service unavailable", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13344")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if err := h.adminAccount.PerformPasswordReset(c, form.Token, form.Password); err != nil {
		h.adminApp.Logger().ErrorContext(c, "admin perform password reset", logger.Error(err))
		switch {
		case errors.Is(err, goauth.ErrInvalidToken), errors.Is(err, goauth.ErrExpiredToken),
			errors.Is(err, goauth.ErrResetAlreadyUsed):
			h.adminApp.HTTPManager().WithFlashError(c, "Невалидный токен!")
		case errors.Is(err, goauth.ErrInvalidPassword), errors.Is(err, goauth.ErrCommonPassword):
			h.adminApp.HTTPManager().WithFlashError(c, "Новый пароль не соответствует политике безопасности.")
		default:
			h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13344")
		}
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Успешный сброс пароля!")

	return h.adminApp.HTTPManager().RedirectBack(c)
}

// Logout обрабатывает выход пользователя.
func (h *Handler) Logout(c fiber.Ctx) error {
	if err := h.sessionStoreActions.DelAdminAuth(c); err != nil {
		h.adminApp.Logger().ErrorContext(c, "session store DelAdminAuth", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13342")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
}

func (h *Handler) Profile(c fiber.Ctx) error {
	adminAuth := adminshared.MustGetAdminAuth(c)

	adminUser, err := h.adminRepo.GetByID(c, adminAuth.ID)
	if err != nil || adminUser.ID == 0 {
		h.adminApp.Logger().ErrorContext(c, "admin repo GetByID", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13342")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	var filePreview *uploadhost.File
	if h.previewService != nil && adminUser.GetPreviewFileID() > 0 {
		filePreview, err = h.previewService.LoadPreview(c, adminUser.GetPreviewFileID())
	}
	if err != nil {
		h.adminApp.Logger().ErrorContext(c, "failed to load admin preview", logger.Error(err))
	}
	h.applySessionRoles(&adminUser, adminAuth)

	return h.adminApp.HTTPManager().Render(c, "profile/ViewPage", map[string]any{
		"title":         "Просмотр профиля",
		"data":          adminUser,   //nolint:goconst // required
		"admin_preview": filePreview, //nolint:goconst // required
	})
}

func (h *Handler) ProfileEdit(c fiber.Ctx) error {
	adminAuth := adminshared.MustGetAdminAuth(c)

	adminUser, err := h.adminRepo.GetByID(c, adminAuth.ID)
	if err != nil || adminUser.ID == 0 {
		h.adminApp.Logger().ErrorContext(c, "admin repo GetByID", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13342")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	var filePreview *uploadhost.File
	if h.previewService != nil && adminUser.GetPreviewFileID() > 0 {
		filePreview, err = h.previewService.LoadPreview(c, adminUser.GetPreviewFileID())
	}
	if err != nil {
		h.adminApp.Logger().ErrorContext(c, "failed to load admin preview", logger.Error(err))
	}
	h.applySessionRoles(&adminUser, adminAuth)

	return h.adminApp.HTTPManager().Render(c, "profile/EditPage", map[string]any{
		"title":         "Редактирование профиля",
		"data":          adminUser,
		"admin_preview": filePreview,
	})
}

func (h *Handler) PostProfileEdit(c fiber.Ctx) error {
	var form FormProfileEdit
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.StringIsPresent{Name: "Name", Field: form.Name, Message: "Name не задан или неверный"},
	)
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	adminAuth := adminshared.MustGetAdminAuth(c)

	adminUser, err := h.adminRepo.GetByID(c, adminAuth.ID)
	if err != nil || adminUser.ID == 0 {
		h.adminApp.Logger().ErrorContext(c, "admin repo GetByID", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13342")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	adminUser.Username = form.Username
	adminUser.Name = form.Name
	adminUser.LastName = form.LastName

	if h.adminAccount == nil {
		h.adminApp.Logger().ErrorContext(c, "admin auth Runtime unavailable", logger.Error(errors.New("nil admin account service")))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13342")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	if _, err := h.adminAccount.UpdateBasicProfile(c, currentAdminSubjectID(adminAuth), goauth.BasicProfile{
		Username:    adminUser.Username,
		DisplayName: strings.TrimSpace(adminUser.Name + " " + adminUser.LastName),
		GivenName:   adminUser.Name, FamilyName: adminUser.LastName,
	}); err != nil {
		h.adminApp.Logger().ErrorContext(c, "admin subjects UpdateProfile", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13342")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	var filePreview *uploadhost.File
	if h.previewService != nil && adminUser.GetPreviewFileID() > 0 {
		filePreview, err = h.previewService.LoadPreview(c, adminUser.GetPreviewFileID())
		if err != nil {
			h.adminApp.Logger().ErrorContext(c, "failed to load admin preview", logger.Error(err))
		}
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Успешно отредактировано!")

	return h.adminApp.HTTPManager().Render(c, "profile/EditPage", map[string]any{
		"title":         "Редактирование профиля",
		"data":          adminUser,
		"admin_preview": filePreview,
	})
}

type pendingEmailChangeProps struct {
	NewEmail  string    `json:"newEmail"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func profilePendingEmailChange(pending *goauth.PendingEmailChange) *pendingEmailChangeProps {
	if pending == nil {
		return nil
	}

	return &pendingEmailChangeProps{
		NewEmail:  pending.NewDisplayValue,
		ExpiresAt: pending.ExpiresAt,
	}
}

func (h *Handler) ProfileSettings(c fiber.Ctx) error {
	adminAuth := adminshared.MustGetAdminAuth(c)

	adminUser, err := h.adminRepo.GetByID(c, adminAuth.ID)
	if err != nil || adminUser.ID == 0 {
		h.adminApp.Logger().ErrorContext(c, "admin repo GetByID", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Не удалось загрузить профиль администратора.")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	var pending *goauth.PendingEmailChange
	if h.adminAccount != nil && h.adminApp.Enabled("authmail") {
		req, err := h.adminAccount.GetPendingEmailChange(c, currentAdminSubjectID(adminAuth))
		if err != nil && !errors.Is(err, goauth.ErrEmailChangeNotFound) {
			h.adminApp.Logger().ErrorContext(c, "admin email change GetPending", logger.Error(err))
		} else {
			pending = req
		}
	}

	return h.adminApp.HTTPManager().Render(c, "profile/SettingsPage", map[string]any{
		"title":              "Настройки",
		"data":               adminUser,
		"pendingEmailChange": profilePendingEmailChange(pending),
	})
}

func (h *Handler) PostProfileChangePassword(c fiber.Ctx) error {
	var form FormProfileChangePassword
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.StringIsPresent{
			Name: "CurrentPassword", Field: form.CurrentPassword, Message: "Введите текущий пароль",
		},
		&validators.StringIsPresent{Name: "Password", Field: form.Password, Message: "Введите новый пароль"},
		&validators.StringIsPresent{
			Name: "ConfirmPassword", Field: form.ConfirmPassword, Message: "Повторите новый пароль",
		},
	)
	if form.Password != form.ConfirmPassword {
		errs.Add("passwordConfirmation", "Пароли не совпадают")
	}
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if h.adminAccount == nil {
		err := errors.New("admin account service is not configured")
		h.adminApp.Logger().ErrorContext(c, "admin account service unavailable", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Не удалось обновить пароль.")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	adminAuth := adminshared.MustGetAdminAuth(c)
	subjectID := currentAdminSubjectID(adminAuth)
	if err := h.adminAccount.ChangePassword(
		c,
		subjectID,
		form.CurrentPassword,
		form.Password,
	); err != nil {
		h.adminApp.Logger().ErrorContext(c, "admin change password", logger.Error(err))
		switch {
		case errors.Is(err, goauth.ErrCurrentPasswordInvalid):
			h.adminApp.HTTPManager().WithFlashError(c, "Неверный текущий пароль.")
		case errors.Is(err, goauth.ErrPasswordUnchanged):
			h.adminApp.HTTPManager().WithFlashError(c, "Новый пароль совпадает с текущим.")
		case errors.Is(err, goauth.ErrInvalidPassword), errors.Is(err, goauth.ErrCommonPassword):
			h.adminApp.HTTPManager().WithFlashError(c, "Новый пароль не соответствует политике безопасности.")
		default:
			h.adminApp.HTTPManager().WithFlashError(c, "Не удалось обновить пароль.")
		}
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if err := h.sessionStoreActions.DelAdminAuth(c); err != nil {
		h.adminApp.Logger().WarnContext(c, "clear browser session after password change", logger.Error(err))
	}
	h.adminApp.HTTPManager().WithFlashSuccess(c, "Пароль обновлён. Войдите снова.")
	return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
}

func (h *Handler) PostProfileEmailRequest(c fiber.Ctx) error {
	if h.adminAccount == nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Сервис смены email недоступен.")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	var form FormProfileEmailRequest
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.EmailIsPresent{Name: "Email", Field: form.Email, Message: "Введите корректный email"},
	)
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	newEmail := strings.TrimSpace(form.Email)
	subjectID := currentAdminSubjectID(adminshared.MustGetAdminAuth(c))
	if err := h.adminAccount.RequestEmailChangeWithPassword(c, subjectID, form.CurrentPassword, newEmail); err != nil {
		h.adminApp.Logger().ErrorContext(c, "email change request", logger.Error(err))
		switch {
		case errors.Is(err, goauth.ErrCurrentPasswordInvalid):
			h.adminApp.HTTPManager().WithValidationErrors(c, map[string][]string{"currentPassword": {"Текущий пароль указан неверно."}})
		case errors.Is(err, goauth.ErrAuthenticationRateLimited):
			h.adminApp.HTTPManager().WithFlashError(c, "Слишком много попыток. Попробуйте позже.")
		case errors.Is(err, goauth.ErrEmailChangeSameValue):
			h.adminApp.HTTPManager().WithFlashError(c, "Новый email совпадает с текущим.")
		case errors.Is(err, goauth.ErrIdentifierAlreadyExists):
			h.adminApp.HTTPManager().WithFlashError(c, "Этот email уже используется другим администратором.")
		case errors.Is(err, goauth.ErrInvalidIdentifier):
			h.adminApp.HTTPManager().WithFlashError(c, "Введите корректный email.")
		case errors.Is(err, goauth.ErrConfirmationResendDelay), errors.Is(err, goauth.ErrConfirmationRateLimited):
			h.adminApp.HTTPManager().WithFlashError(c, "Пожалуйста, подождите перед повторной отправкой кода.")
		default:
			h.adminApp.HTTPManager().WithFlashError(c, "Не удалось отправить код подтверждения.")
		}
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Код подтверждения отправлен на новый email.")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) PostProfileEmailConfirm(c fiber.Ctx) error {
	if h.adminAccount == nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Сервис смены email недоступен.")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	var form FormProfileEmailConfirm
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.StringIsPresent{Name: "Code", Field: form.Code, Message: "Введите код подтверждения"},
	)
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithValidationErrors(c, errs.Errors)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	adminAuth := adminshared.MustGetAdminAuth(c)
	_, err := h.adminAccount.ConfirmEmailChange(c, currentAdminSubjectID(adminAuth), form.Code)
	if err != nil {
		h.adminApp.Logger().ErrorContext(c, "email change confirm", logger.Error(err))
		switch {
		case errors.Is(err, goauth.ErrEmailChangeNotFound):
			h.adminApp.HTTPManager().WithFlashError(c, "Запрос на смену email не найден.")
		case errors.Is(err, goauth.ErrInvalidConfirmationCode):
			h.adminApp.HTTPManager().WithFlashError(c, "Неверный код подтверждения.")
		case errors.Is(err, goauth.ErrConfirmationExpired):
			h.adminApp.HTTPManager().WithFlashError(c, "Срок действия кода истёк. Запросите новый.")
		case errors.Is(err, goauth.ErrConfirmationAttempts):
			h.adminApp.HTTPManager().WithFlashError(c, "Превышено число попыток. Запросите новый код.")
		default:
			h.adminApp.HTTPManager().WithFlashError(c, "Не удалось подтвердить смену email.")
		}
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Email успешно обновлён.")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) applySessionRoles(adminUser *models2.Admin, adminSession *models2.SessionAdmin) {
	if adminUser == nil || adminSession == nil {
		return
	}

	if len(adminSession.Roles) == 0 {
		return
	}

	if len(adminUser.GetRoles()) > 0 {
		return
	}

	data := adminUser.GetData()
	data.Roles = append([]string(nil), adminSession.Roles...)
	adminUser.Data = data
}

func currentAdminSubjectID(adminAuth *models2.SessionAdmin) authcore.SubjectID {
	return adminAuth.AuthSubjectID()
}

func (h *Handler) touchAdminLoginMetadata(ctx context.Context, adminID int64, lastLoginAt time.Time) {
	if adminID <= 0 {
		return
	}

	adminUser, err := h.adminRepo.GetByID(ctx, adminID)
	if err != nil || adminUser.ID == 0 {
		if err != nil {
			h.adminApp.Logger().WarnContext(ctx, "admin repo GetByID for auth metadata", logger.Error(err))
		}
		return
	}

	data := adminUser.GetData()
	loggedAt := lastLoginAt.UTC()
	data.LastLoginAt = &loggedAt
	adminUser.Data = data

	if err := h.adminRepo.UpdateAdmin(ctx, adminUser.ID, adminUser); err != nil {
		h.adminApp.Logger().WarnContext(ctx, "admin repo UpdateAdmin auth metadata", logger.Error(err))
	}
}

func (h *Handler) buildPostLoginRedirect(c fiber.Ctx, adminID int64) string {
	target := strings.TrimSpace(c.Query("redirectback"))
	if target == "" {
		target = h.extractRedirectFromReferer(c.Get(fiber.HeaderReferer))
	}

	cleaned := h.normalizeRedirectTarget(target)
	redirectToHome := false
	if cleaned != "" {
		targetURL, _ := url.Parse(cleaned)
		redirectToHome = targetURL.Path == "/"
		if targetURL.Path != "/" && targetURL.Path != "/auth/login" && targetURL.Path != "/auth/register" {
			return cleaned
		}
	}

	if h.adminApp == nil || h.adminApp.RolesService() == nil {
		return "/auth/profile"
	}

	admin := &models2.SessionAdmin{ID: adminID}
	allowedMenu := menu.BuildAdminMenu(c, admin, h.adminApp.RolesService(), h.adminMenu)
	for _, section := range allowedMenu.Sections {
		if destination := firstMenuDestination(section.Items); destination != "" {
			if destination == "/" && redirectToHome {
				return cleaned
			}
			return destination
		}
	}

	return "/auth/profile"
}

func firstMenuDestination(items []menu.Item) string {
	for _, item := range items {
		if strings.HasPrefix(item.Href, "/") && !strings.HasPrefix(item.Href, "//") {
			return item.Href
		}
		if destination := firstMenuDestination(item.Children); destination != "" {
			return destination
		}
	}
	return ""
}

func (h *Handler) extractRedirectFromReferer(referer string) string {
	if referer == "" {
		return ""
	}

	refURL, err := url.Parse(referer)
	if err != nil {
		return ""
	}

	return refURL.Query().Get("redirectback")
}

func (h *Handler) normalizeRedirectTarget(value string) string {
	if value == "" {
		return ""
	}

	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return ""
	}

	targetURL, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if strings.Contains(targetURL.Path, "\\") || strings.HasPrefix(targetURL.Path, "//") {
		return ""
	}

	// Absolute URL: allow only same admin host.
	if targetURL.Scheme != "" || targetURL.Host != "" {
		if targetURL.Scheme == "" || targetURL.Host == "" {
			return ""
		}
		if !h.isSameAdminHost(targetURL) {
			return ""
		}
		return buildRedirectPath(targetURL)
	}

	// Relative URL should start with "/" and have no host.
	if targetURL.Host != "" || targetURL.Path == "" {
		return ""
	}

	cleanPath := path.Clean(targetURL.Path)
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	targetURL.Path = cleanPath
	return buildRedirectPath(targetURL)
}

func (h *Handler) isSameAdminHost(target *url.URL) bool {
	if target == nil {
		return false
	}

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(target.Host, baseURL.Host)
}

func buildRedirectPath(u *url.URL) string {
	if u == nil {
		return ""
	}

	targetPath := u.Path
	if targetPath == "" {
		targetPath = "/"
	}

	if u.RawQuery != "" {
		targetPath += "?" + u.RawQuery
	}

	if u.Fragment != "" {
		targetPath += "#" + u.Fragment
	}

	return targetPath
}

func (h *Handler) recordSuccessfulLogin(
	ctx context.Context,
	subjectID goauth.SubjectID,
	adminUser models2.Admin,
) {
	if h.loginAudit == nil {
		return
	}

	record := adminloginaudit.Record{
		LoggedAt:  time.Now().UTC(),
		AdminID:   adminUser.ID,
		AdminUUID: adminUser.UUID,
		SubjectID: subjectID,
		Email:     adminUser.Email,
		Name:      adminUser.Name,
		LastName:  adminUser.LastName,
		Username:  adminUser.Username,
		IP:        clientIPFromContext(ctx),
		UserAgent: userAgentFromContext(ctx),
		RequestID: requestid.FromContext(ctx),
	}

	if err := h.loginAudit.RecordSuccessfulLogin(ctx, record); err != nil {
		h.adminApp.Logger().WarnContext(ctx, "record admin login audit", logger.Error(err))
	}
}

func clientIPFromContext(ctx context.Context) string {
	c, ok := ctx.(fiber.Ctx)
	if !ok {
		return ""
	}

	return strings.TrimSpace(c.IP())
}

func userAgentFromContext(ctx context.Context) string {
	c, ok := ctx.(fiber.Ctx)
	if !ok {
		return ""
	}

	return strings.TrimSpace(c.Get(fiber.HeaderUserAgent))
}
