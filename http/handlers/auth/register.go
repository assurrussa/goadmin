package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	"github.com/gobuffalo/validate/validators"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/core/formvalidator"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/authdelay"
	identity "github.com/assurrussa/goadmin/internal/identity"
	models2 "github.com/assurrussa/goadmin/models"
)

var errFirstAdminAlreadyExists = errors.New("first admin bootstrap is closed")

const (
	firstAdminRoleDescription = "Полный доступ ко всем действиям админ-панели"
	firstAdminRoleName        = "Супер-администратор"
	firstAdminFallbackName    = "Admin"
	firstAdminFallbackUser    = "admin"
)

type adminBootstrapRepo interface {
	GetList(ctx context.Context, filters datagrid.Filtered) ([]models2.Admin, int, error)
}

type rolesBootstrapManager interface {
	ListRoles(ctx context.Context, filter integrationroles.RoleFilter) ([]integrationroles.Role, error)
	EnsurePermissions(ctx context.Context, inputs []integrationroles.CreatePermissionInput) error
	CreatePermissions(ctx context.Context, inputs []integrationroles.CreatePermissionInput) ([]integrationroles.Permission, error)
	CreateRole(ctx context.Context, input integrationroles.CreateRoleInput) (*integrationroles.Role, error)
	UpdateRole(ctx context.Context, input integrationroles.UpdateRoleInput) (*integrationroles.Role, error)
	AssignRolesToSubject(ctx context.Context, subjectID string, roleIDs []int64) error
}

type FormRegister struct {
	Name            string `json:"name"`
	LastName        string `json:"lastName"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"passwordConfirm"`
	SetupToken      string `json:"setupToken"`
}

func (h *Handler) GetRegister(c fiber.Ctx) error {
	available, err := h.firstAdminRegistrationAvailable(c)
	if err != nil {
		h.adminApp.Logger().ErrorContext(c, "check first admin bootstrap", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Не удалось открыть регистрацию первого администратора.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}
	if !available {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Первый администратор уже создан.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}
	if h.setupTokens == nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Регистрация первого администратора недоступна.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}

	return h.adminApp.HTTPManager().Render(c, "auth/RegisterPage", map[string]any{
		"title": "Регистрация первого администратора", //nolint:goconst // required
	})
}

func (h *Handler) PostRegister(c fiber.Ctx) error {
	var form FormRegister
	if err := c.Bind().Body(&form); err != nil {
		return err
	}

	h.adminApp.HTTPManager().WithFlashOld(c, map[string]any{
		"name":     form.Name,
		"lastName": form.LastName,
		"username": form.Username,
		"email":    form.Email,
	})

	authdelay.RunSleeper(c, h.sleep)

	errs := formvalidator.Validate(
		&validators.StringIsPresent{Name: "Name", Field: form.Name, Message: "Имя не задано"},
		&validators.EmailIsPresent{Name: "Email", Field: form.Email, Message: "Email не задан или неверный"}, //nolint:goconst,lll // required
		&validators.StringIsPresent{Name: "Password", Field: form.Password, Message: "Пароль не задан"},      //nolint:goconst,lll // required
		&validators.StringIsPresent{
			Name: "SetupToken", Field: form.SetupToken, Message: "Setup-токен не задан",
		},
		&validators.StringsMatch{
			Name: "PasswordConfirm", Field: form.PasswordConfirm, Field2: form.Password,
			Message: "Пароли не совпадают",
		},
	)
	if len(errs.Errors) > 0 {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Форма заполнена неверно.")
		return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, errs.Errors)
	}

	if _, err := h.getBootstrapRepo(); err != nil {
		h.adminApp.Logger().ErrorContext(c, "bootstrap repo unavailable", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Регистрация первого администратора недоступна.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}
	if h.rolesManager == nil {
		h.adminApp.Logger().ErrorContext(c, "roles manager unavailable", logger.Error(errors.New("nil roles manager")))
		h.adminApp.HTTPManager().WithFlashError(c, "Регистрация первого администратора недоступна.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}
	if h.setupTokens == nil {
		h.adminApp.Logger().ErrorContext(c, "first admin setup unavailable", logger.Error(errors.New("nil setup token service")))
		h.adminApp.HTTPManager().WithFlashError(c, "Регистрация первого администратора недоступна.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}
	if err := h.setupTokens.Validate(c, form.SetupToken); err != nil {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Setup-токен недействителен или истек.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}

	available, err := h.firstAdminRegistrationAvailable(c)
	if err != nil {
		h.adminApp.Logger().ErrorContext(c, "check first admin bootstrap", logger.Error(err))
		message := "Не удалось создать первого администратора."
		if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
			message = "Результат операции неизвестен. Проверьте возможность входа перед повтором."
		}
		h.adminApp.HTTPManager().WithFlashError(c, message)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	if !available {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Первый администратор уже создан.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}

	prepared, err := h.prepareFirstAdmin(c, form)
	if err == nil {
		err = h.adminApp.InTransaction(c, func(ctx context.Context) error {
			return h.createFirstAdmin(ctx, form, prepared)
		})
	}
	if err != nil {
		if errors.Is(err, errFirstAdminAlreadyExists) {
			h.adminApp.HTTPManager().WithFlashWarning(c, "Первый администратор уже создан.")
			return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
		}
		if errors.Is(err, goauth.ErrInvalidCredentials) {
			h.adminApp.HTTPManager().WithFlashWarning(c, "Не удалось подтвердить существующий аккаунт.")
			return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, goinertia.ValidationErrors{
				"password": []string{"Пароль не подходит к существующему аккаунту"},
			})
		}
		if errors.Is(err, goauth.ErrInvalidPassword) || errors.Is(err, goauth.ErrCommonPassword) {
			h.adminApp.HTTPManager().WithFlashWarning(c, "Невалидный пароль!")
			return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, goinertia.ValidationErrors{
				"password": []string{"Невалидный пароль"},
			})
		}

		h.adminApp.Logger().ErrorContext(c, "register first admin", logger.Error(err))
		message := "Не удалось создать первого администратора."
		if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
			message = "Результат операции неизвестен. Проверьте возможность входа перед повтором."
		}
		h.adminApp.HTTPManager().WithFlashError(c, message)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if _, _, err := h.sessionStoreActions.LoginAdmin(c, form.Email, form.Password, false); err != nil {
		h.adminApp.Logger().ErrorContext(c, "bootstrap session auth", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashSuccess(c, "Первый администратор создан. Выполните вход.")
		return h.adminApp.HTTPManager().Redirect(c, "/auth/login")
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Первый администратор создан.")
	return h.adminApp.HTTPManager().Redirect(c, "/")
}

func (h *Handler) createFirstAdmin(ctx context.Context, form FormRegister,
	prepared ...func(context.Context) (goauth.Account, error),
) error {
	allowed, err := h.firstAdminRegistrationAvailable(ctx)
	if err != nil {
		return fmt.Errorf("check first admin bootstrap: %w", err)
	}
	if !allowed {
		return errFirstAdminAlreadyExists
	}
	if err := h.setupTokens.Consume(ctx, form.SetupToken); err != nil {
		return fmt.Errorf("consume first admin setup token: %w", err)
	}
	if h.sessionStoreActions == nil {
		return errors.New("admin auth Runtime is not configured")
	}

	admin := models2.Admin{
		UUID:     identity.NewUserID(),
		Username: normalizeInitialAdminUsername(form.Username, form.Email),
		Name:     normalizeInitialAdminName(form.Name),
		LastName: strings.TrimSpace(form.LastName),
	}
	var account goauth.Account
	if len(prepared) > 0 {
		account, err = prepared[0](ctx)
	} else {
		account, err = h.sessionStoreActions.ProvisionTrustedAdmin(ctx, goauth.RegisterRequest{
			Email: form.Email, Password: form.Password,
			Profile: goauth.BasicProfile{
				Username: admin.Username, DisplayName: strings.TrimSpace(admin.Name + " " + admin.LastName),
				GivenName: admin.Name, FamilyName: admin.LastName,
			},
		})
	}
	if err != nil {
		return fmt.Errorf("create first admin: %w", err)
	}

	roleID, err := h.ensureFirstAdminRole(ctx)
	if err != nil {
		return fmt.Errorf("ensure first admin role: %w", err)
	}
	if err := h.rolesManager.AssignRolesToSubject(ctx, account.Subject.ID.String(), []int64{roleID}); err != nil {
		return fmt.Errorf("assign first admin role: %w", err)
	}
	created, err := h.adminRepo.ProvisionAccount(ctx, account, admin.UUID)
	if err != nil {
		return fmt.Errorf("provision first admin membership: %w", err)
	}

	if h.actionAudit == nil {
		return errors.New("first admin audit writer is required")
	}
	return h.actionAudit.RecordAdminAction(ctx, adminactionauditrepo.Record{
		ActorAdminID: created.ID, ActorSubjectID: account.Subject.ID.String(), Action: "admin.first_created",
		TargetType: "admin", TargetID: strconv.FormatInt(created.ID, 10),
	})
}

func (h *Handler) ensureFirstAdminRole(ctx context.Context) (int64, error) {
	if h.rolesManager == nil {
		return 0, errors.New("roles manager is nil")
	}

	catalog := integrationroles.NewPermissionCatalog(h.adminApp.PermissionDefinitions())
	permissionsInput := make([]integrationroles.CreatePermissionInput, 0, len(catalog.Data))
	for _, def := range catalog.Data {
		permissionsInput = append(permissionsInput, integrationroles.CreatePermissionInput(def))
	}

	if err := h.rolesManager.EnsurePermissions(ctx, permissionsInput); err != nil {
		return 0, err
	}

	permissions := catalog.GetPermissions()
	existing, err := h.rolesManager.ListRoles(ctx, integrationroles.RoleFilter{
		Slugs:      []string{integrationroles.SuperAdminRole},
		IncludeSys: true,
	})
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		isSystem := true
		updated, err := h.rolesManager.UpdateRole(ctx, integrationroles.UpdateRoleInput{
			ID:          existing[0].ID,
			Slug:        integrationroles.SuperAdminRole,
			Name:        firstAdminRoleName,
			Description: stringPtr(firstAdminRoleDescription),
			IsSystem:    &isSystem,
			Permissions: &permissions,
		})
		if err != nil {
			return 0, err
		}

		return updated.ID, nil
	}

	role, err := h.rolesManager.CreateRole(ctx, integrationroles.CreateRoleInput{
		Slug:        integrationroles.SuperAdminRole,
		Name:        firstAdminRoleName,
		Description: stringPtr(firstAdminRoleDescription),
		IsSystem:    true,
		Permissions: permissions,
	})
	if err != nil {
		return 0, err
	}

	return role.ID, nil
}

func (h *Handler) firstAdminRegistrationAvailable(ctx context.Context) (bool, error) {
	bootstrapRepo, err := h.getBootstrapRepo()
	if err != nil {
		return false, err
	}

	_, total, err := bootstrapRepo.GetList(ctx, datagrid.Filters{
		Page:  1,
		Limit: 1,
	})
	if err != nil {
		return false, err
	}

	return total == 0, nil
}

func (h *Handler) getBootstrapRepo() (adminBootstrapRepo, error) {
	bootstrapRepo, ok := h.adminRepo.(adminBootstrapRepo)
	if !ok {
		return nil, errors.New("admin bootstrap repo is not configured")
	}

	return bootstrapRepo, nil
}

func normalizeInitialAdminName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return firstAdminFallbackName
	}

	return name
}

func normalizeInitialAdminUsername(username, email string) string {
	username = strings.TrimSpace(username)
	if username != "" {
		return username
	}

	localPart := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	if localPart != "" {
		return localPart
	}

	return firstAdminFallbackUser
}

func stringPtr(v string) *string {
	value := v
	return &value
}

func (h *Handler) prepareFirstAdmin(
	ctx context.Context, form FormRegister,
) (func(context.Context) (goauth.Account, error), error) {
	if h.sessionStoreActions == nil {
		return nil, errors.New("admin auth Runtime is not configured")
	}
	request := goauth.RegisterRequest{Email: form.Email, Password: form.Password, Profile: goauth.BasicProfile{
		Username:  normalizeInitialAdminUsername(form.Username, form.Email),
		GivenName: normalizeInitialAdminName(form.Name), FamilyName: strings.TrimSpace(form.LastName),
		DisplayName: strings.TrimSpace(normalizeInitialAdminName(form.Name) + " " + strings.TrimSpace(form.LastName)),
	}}
	if preparer, ok := h.sessionStoreActions.(interface {
		PrepareTrustedAdmin(ctx context.Context, request goauth.RegisterRequest) (func(context.Context) (goauth.Account, error), error)
	}); ok {
		return preparer.PrepareTrustedAdmin(ctx, request)
	}
	return func(ctx context.Context) (goauth.Account, error) {
		return h.sessionStoreActions.ProvisionTrustedAdmin(ctx, request)
	}, nil
}
