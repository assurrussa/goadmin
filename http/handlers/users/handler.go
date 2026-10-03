package users

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	authcore "github.com/assurrussa/goadmin/internal/auth"
)

//go:generate toolsmocks

const (
	maxExportRows  = 10000
	userEmailField = "email"
)

type userRepo interface {
	GetList(ctx context.Context, filters datagrid.Filtered) ([]authcore.Profile, int, error)
	GetByID(ctx context.Context, id int64) (authcore.Profile, error)
	Update(ctx context.Context, id int64, user authcore.Profile) error
	SoftDeleteByID(ctx context.Context, id int64) error
	RestoreByID(ctx context.Context, id int64) error
}

type subjectEmailStore interface {
	UpdateBasicProfile(
		ctx context.Context,
		subjectID goauth.SubjectID,
		profile goauth.BasicProfile,
	) (goauth.Account, error)
	RequestEmailChange(ctx context.Context, subjectID goauth.SubjectID, newEmail string) error
	SetSubjectStatus(
		ctx context.Context,
		subjectID goauth.SubjectID,
		status goauth.SubjectStatus,
	) (goauth.Subject, error)
}

type Handler struct {
	adminApp        *adminapp.App
	routePath       string
	dataGridHandler *datagrid.Handler[authcore.Profile]
	userRepo        userRepo
	subjects        subjectEmailStore
}

func NewHandler(
	adminApp *adminapp.App,
	userRepo userRepo,
	subjects subjectEmailStore,
) *Handler {
	routePath := "/users"
	config := datagrid.FilterConfig[authcore.Profile]{
		RoutePath:         routePath,
		Entity:            "user",
		Title:             "Пользователи",
		Repository:        userRepo,
		DefaultSort:       "id",
		DefaultOrder:      "desc",
		PageSize:          20,
		Creatable:         false,
		Exportable:        true,
		Refreshable:       true,
		CreateButtonText:  "Создать пользователя",
		EmptyMessage:      "Пользователи не найдены",
		SearchPlaceholder: "Поиск по email, имени...",
		IDKey:             "id",
		Columns: []datagrid.Column{
			{
				Key:        "status",
				Label:      "Статус",
				Type:       "select", //nolint:goconst // required
				Filterable: true,
				Badges: map[string]map[string]string{
					"active":  {"label": "Активный", "variant": "success"}, //nolint:goconst // required
					"deleted": {"label": "Удалён", "variant": "danger"},    //nolint:goconst // required
				},
				FilterOptions: []map[string]string{
					{"value": "all", "label": "Все"}, //nolint:goconst // required
					{"value": "active", "label": "Активные"},
					{"value": "deleted", "label": "Удалённые"},
				},
			},
			{
				Key:        "emailStatus",
				Label:      "Email статус",
				Type:       "select",
				Filterable: true,
				Badges: map[string]map[string]string{
					"confirmed":   {"label": "Подтверждён", "variant": "success"},    //nolint:goconst // required
					"unconfirmed": {"label": "Не подтверждён", "variant": "warning"}, //nolint:goconst // required
				},
				FilterOptions: []map[string]string{
					{"value": "any", "label": "Любой"},
					{"value": "confirmed", "label": "Подтверждён"},
					{"value": "unconfirmed", "label": "Не подтверждён"},
				},
			},
			{
				Key:        "phoneStatus",
				Label:      "Телефон статус",
				Type:       "select",
				Filterable: true,
				Badges: map[string]map[string]string{
					"confirmed":   {"label": "Подтверждён", "variant": "success"},
					"unconfirmed": {"label": "Не подтверждён", "variant": "warning"},
				},
				FilterOptions: []map[string]string{
					{"value": "any", "label": "Любой"},
					{"value": "confirmed", "label": "Подтверждён"},
					{"value": "unconfirmed", "label": "Не подтверждён"},
				},
			},
			{
				Key:        "id",
				Label:      "ID",
				Type:       "number",
				Sortable:   true,
				Filterable: false,
			},
			{
				Key:               userEmailField,
				Label:             "Email",
				Type:              "text", //nolint:goconst // required
				Sortable:          true,
				Filterable:        true,
				FilterPlaceholder: "Введите email",
			},
			{
				Key:        "username", //nolint:goconst // required
				Label:      "Логин",
				Type:       "text",
				Sortable:   true,
				Filterable: true,
			},
			{
				Key:        "name", //nolint:goconst // required
				Label:      "Имя",
				Type:       "text",
				Sortable:   true,
				Filterable: true,
			},
			{
				Key:        "lastName", //nolint:goconst // required
				Label:      "Фамилия",
				Type:       "text",
				Sortable:   true,
				Filterable: true,
			},
			{
				Key:        "phone", //nolint:goconst // required
				Label:      "Телефон",
				Type:       "text",
				Sortable:   false,
				Filterable: false,
			},
			{
				Key:        "confirmedEmailAt",
				Label:      "Email подтвержден",
				Type:       "date",             //nolint:goconst // required
				Format:     "2006-01-02 15:04", //nolint:goconst // required
				Sortable:   true,
				Filterable: false,
			},
			{
				Key:        "confirmedPhoneAt",
				Label:      "Телефон подтвержден",
				Type:       "date",
				Format:     "2006-01-02 15:04",
				Sortable:   true,
				Filterable: false,
			},
			{
				Key:        "createdAt",
				Label:      "Создан",
				Type:       "date",
				Format:     "2006-01-02 15:04",
				Sortable:   true,
				Filterable: true,
			},
		},
		Actions: []datagrid.Action[authcore.Profile]{
			{Key: "view", Label: "Просмотр", Icon: "eye", Variant: "secondary"},
			{Key: "edit", Label: "Редактировать", Icon: "edit", Variant: "primary"},
			{Key: "delete", Label: "Удалить", Icon: "trash", Variant: "danger", CanView: func(_ context.Context, u authcore.Profile) bool {
				return !u.DeletedAt.Valid
			}},
			{
				Key:     "restore",
				Label:   "Восстановить",
				Icon:    "rotate",
				Variant: "info",
				CanView: func(_ context.Context, u authcore.Profile) bool {
					return u.DeletedAt.Valid
				},
			},
		},
		RowDecorators: []datagrid.RowDecorator[authcore.Profile]{
			datagrid.NewRowDecorator(
				nil,
				func(_ context.Context, profile authcore.Profile) (map[string]any, error) {
					status := "active"
					if profile.DeletedAt.Valid {
						status = "deleted"
					}

					emailStatus := "unconfirmed"
					if profile.IsEmailConfirmed() {
						emailStatus = "confirmed"
					}

					phoneStatus := "unconfirmed"
					if profile.IsPhoneConfirmed() {
						phoneStatus = "confirmed"
					}

					return map[string]any{
						"status":      status,
						"emailStatus": emailStatus,
						"phoneStatus": phoneStatus,
						"username":    valueOrDash(profile.Username),
						"name":        valueOrDash(&profile.Name),
						"lastName":    valueOrDash(profile.LastName),
						"phone":       phoneOrDash(profile.Phone),
					}, nil
				},
			),
		},
		PageComponent: func(c fiber.Ctx, data datagrid.Response[authcore.Profile]) error {
			dataMap := data.ToAPIResponse()

			return adminApp.HTTPManager().Render(c, "users/IndexPage", map[string]any{
				"title": data.Title, //nolint:goconst // required
				"data":  dataMap,    //nolint:goconst // required
			})
		},
	}

	return &Handler{
		adminApp:        adminApp,
		routePath:       routePath,
		userRepo:        userRepo,
		subjects:        subjects,
		dataGridHandler: datagrid.NewHandler(config, adminApp.Logger()),
	}
}

func valueOrDash(value *string) string {
	if value == nil || *value == "" {
		return "—"
	}

	return *value
}

func phoneOrDash(phone *int64) string {
	if phone == nil {
		return "—"
	}

	return strconv.FormatInt(*phone, 10)
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	groupGuard := route.Group(h.routePath, h.adminApp.Guard(authcore.PermissionDomainUsers, authcore.PermissionActionRead)) //nolint:lll // required
	h.dataGridHandler.RegisterRoutes(groupGuard, "")
	groupGuard.Get("export", h.Export)
	groupGuard.Post("refresh", h.Refresh)
	groupGuard.Get(":id", h.View)
	groupGuard.Get(":id/edit", h.adminApp.Guard(authcore.PermissionDomainUsers, authcore.PermissionActionUpdate), h.Edit)        //nolint:lll // required
	groupGuard.Put(":id", h.adminApp.Guard(authcore.PermissionDomainUsers, authcore.PermissionActionUpdate), h.Update)           //nolint:lll // required
	groupGuard.Delete(":id", h.adminApp.Guard(authcore.PermissionDomainUsers, authcore.PermissionActionDelete), h.Delete)        //nolint:lll // required
	groupGuard.Post(":id/restore", h.adminApp.Guard(authcore.PermissionDomainUsers, authcore.PermissionActionDelete), h.Restore) //nolint:lll // required
}

// View показывает страницу пользователя.
func (h *Handler) View(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("user handler view: %w", err)
	}

	user, err := h.userRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("user handler view: get by id: %w", err)
	}

	return h.adminApp.HTTPManager().Render(c, "users/ViewPage", map[string]any{
		"title": "Профиль пользователя",
		"data":  user,
	})
}

// Edit форма редактирования пользователя.
func (h *Handler) Edit(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("user handler edit: %w", err)
	}

	user, err := h.userRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("user handler edit: get by id: %w", err)
	}

	return h.adminApp.HTTPManager().Render(c, "users/EditPage", map[string]any{
		"title": "Редактирование пользователя",
		"data":  user,
	})
}

type updateUserInput struct {
	Name     string  `json:"name"`
	LastName *string `json:"lastName"`
	Username *string `json:"username"`
	Email    string  `json:"email"`
}

// Update обновляет пользователя.
func (h *Handler) Update(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("user handler update: %w", err)
	}

	var form updateUserInput
	if err := c.Bind().Body(&form); err != nil {
		return fmt.Errorf("user handler update parse: %w", err)
	}

	err = h.adminApp.InTransaction(c, func(ctx context.Context) error {
		return h.updateUser(ctx, id, form)
	})
	if err != nil {
		old := map[string]any{
			"name": form.Name, "lastName": form.LastName, "username": form.Username,
		}
		// A readonly email must recover from canonical edit-page data, not rejected input.
		if h.adminApp.Enabled("authmail") {
			old[userEmailField] = form.Email
		}
		h.adminApp.HTTPManager().WithFlashOld(c, old)
		if errors.Is(err, goauth.ErrNotificationDeliveryDisabled) {
			return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, goinertia.ValidationErrors{
				userEmailField: {"Изменение электронной почты отключено"},
			})
		}
		var httpErr *fiber.Error
		if errors.As(err, &httpErr) && httpErr.Code == fiber.StatusNotFound {
			return httpErr
		}
		if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
			h.adminApp.HTTPManager().WithFlashError(c, "Результат операции неизвестен. Обновите страницу перед повтором.")
			return h.adminApp.HTTPManager().RedirectBack(c)
		}
		return fmt.Errorf("user handler update: %w", err)
	}
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) updateUser(ctx context.Context, id int64, form updateUserInput) error {
	current, err := h.userRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}
	if current.ID <= 0 {
		return fiber.NewError(fiber.StatusNotFound, "Пользователь не найден")
	}
	emailChanged := form.Email != "" && form.Email != current.Email
	if emailChanged && !h.adminApp.Enabled("authmail") {
		return goauth.ErrNotificationDeliveryDisabled
	}
	if h.subjects == nil {
		return errors.New("canonical profile Runtime is required")
	}
	current.Name = form.Name
	current.LastName = form.LastName
	current.Username = form.Username
	subjectID := userSubjectID(current)
	if _, err := h.subjects.UpdateBasicProfile(ctx, subjectID, goauth.BasicProfile{
		Username: valueOrEmpty(form.Username), DisplayName: displayName(current.Name, form.LastName),
		GivenName: current.Name, FamilyName: valueOrEmpty(form.LastName),
	}); err != nil {
		return fmt.Errorf("update canonical profile: %w", err)
	}
	if err := h.userRepo.Update(ctx, id, current); err != nil {
		return fmt.Errorf("update user projection: %w", err)
	}
	if emailChanged {
		if err := h.subjects.RequestEmailChange(ctx, subjectID, form.Email); err != nil {
			return fmt.Errorf("request canonical email change: %w", err)
		}
	}
	return nil
}

func userSubjectID(profile authcore.Profile) authcore.SubjectID {
	return profile.AuthSubjectID()
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func displayName(given string, family *string) string {
	return strings.TrimSpace(given + " " + valueOrEmpty(family))
}

// Delete выполняет Soft-Delete пользователя.
func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("user handler delete: %w", err)
	}

	user, err := h.userRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("user handler delete: load subject: %w", err)
	}
	if _, err := h.subjects.SetSubjectStatus(c, user.AuthSubjectID(), goauth.SubjectStatusDisabled); err != nil {
		return fmt.Errorf("user handler delete: disable subject: %w", err)
	}
	if err := h.userRepo.SoftDeleteByID(c, id); err != nil {
		return fmt.Errorf("user handler delete: soft delete: %w", err)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Пользователь помечен как удалён")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

// Restore снимает soft-delete.
func (h *Handler) Restore(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("user handler restore: %w", err)
	}

	user, err := h.userRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("user handler restore: load subject: %w", err)
	}
	if _, err := h.subjects.SetSubjectStatus(c, user.AuthSubjectID(), goauth.SubjectStatusActive); err != nil {
		return fmt.Errorf("user handler restore: activate subject: %w", err)
	}
	if err := h.userRepo.RestoreByID(c, id); err != nil {
		return fmt.Errorf("user handler restore: %w", err)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Пользователь восстановлен")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) getID(c fiber.Ctx) (int64, error) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id is not valid: %v", c.Params("id"))
	}
	return int64(id), nil
}

func (h *Handler) Refresh(c fiber.Ctx) error {
	h.adminApp.HTTPManager().WithFlashInfo(c, "Данные обновлены!")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

// Export экспортирует пользователей в CSV с учетом фильтров.
func (h *Handler) Export(c fiber.Ctx) error {
	cfg := h.dataGridHandler.GetConfig()

	// Парсим фильтры на основе конфигурации
	f := datagrid.ParseFilters(c, datagrid.FilterConfig[any]{
		Entity:       cfg.Entity,
		Title:        cfg.Title,
		Columns:      cfg.Columns,
		DefaultSort:  cfg.DefaultSort,
		DefaultOrder: cfg.DefaultOrder,
		PageSize:     cfg.PageSize,
	})
	f.Page = 1
	f.Limit = maxExportRows

	list, total, err := h.userRepo.GetList(c, f)
	if err != nil {
		return fmt.Errorf("users export get list: %w", err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	// Заголовки
	_ = w.Write([]string{
		"id",
		userEmailField,
		"username",
		"name",
		"lastName",
		"phone",
		"emailConfirmed",
		"phoneConfirmed",
		"createdAt",
		"deletedAt",
	})
	// Данные
	for _, u := range list {
		username := ""
		if u.Username != nil {
			username = *u.Username
		}
		lastName := ""
		if u.LastName != nil {
			lastName = *u.LastName
		}
		phone := ""
		if u.Phone != nil {
			phone = strconv.FormatInt(*u.Phone, 10)
		}
		emailConfirmed := "false"
		if u.ConfirmedEmailAt.Valid {
			emailConfirmed = "true"
		}
		phoneConfirmed := "false"
		if u.ConfirmedPhoneAt.Valid {
			phoneConfirmed = "true"
		}
		deletedAt := ""
		if u.DeletedAt.Valid {
			deletedAt = u.DeletedAt.Time.Format(time.RFC3339)
		}
		_ = w.Write([]string{
			strconv.FormatInt(u.ID, 10),
			u.Email,
			username,
			u.Name,
			lastName,
			phone,
			emailConfirmed,
			phoneConfirmed,
			u.CreatedAt.Format(time.RFC3339),
			deletedAt,
		})
	}
	w.Flush()

	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Goadmin-Export-Truncated", strconv.FormatBool(total > maxExportRows))
	filename := fmt.Sprintf("users-%s.csv", time.Now().Format("20060102"))
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", filename))
	return c.Send(buf.Bytes())
}
