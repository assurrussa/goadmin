package administrations

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gobuffalo/validate/validators"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/jackc/pgx/v5"

	"github.com/assurrussa/goadmin/adminapp"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/core/formvalidator"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/pointer"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/shared"
)

//go:generate toolsmocks

// adminRepo интерфейс репозитория администраторов.
type adminRepo interface {
	ProvisionAccount(
		ctx context.Context,
		account goauth.Account,
		publicID identity.UserID,
	) (models.Admin, error)
	UpdateAdmin(ctx context.Context, id int64, admin models.Admin) error
	GetByUUID(ctx context.Context, id identity.UserID) (models.Admin, error)
	GetList(ctx context.Context, filters datagrid.Filtered) ([]models.Admin, int, error)
	GetByID(ctx context.Context, id int64) (models.Admin, error)
	DeleteByID(ctx context.Context, id int64) error
	SoftDeleteByID(ctx context.Context, id int64) error
}

type adminRoleNamesLister interface {
	ListRoleNamesByAdminIDs(ctx context.Context, adminIDs []int64) (map[int64][]string, error)
}

type adminAuthRuntime interface {
	ProvisionTrustedAdmin(ctx context.Context, request goauth.RegisterRequest) (goauth.Account, error)
	UpdateBasicProfile(
		ctx context.Context,
		subjectID goauth.SubjectID,
		profile goauth.BasicProfile,
	) (goauth.Account, error)
	RequestEmailChange(ctx context.Context, subjectID goauth.SubjectID, newEmail string) error
}

type fileRepo interface {
	GetByID(ctx context.Context, id int64) (uploadhost.File, error)
	Update(ctx context.Context, id int64, file uploadhost.File) error
}

type previewService interface {
	LoadPreview(ctx context.Context, fileID int64) (*uploadhost.File, error)
}

// Handler обработчик для администраторов.
type Handler struct {
	adminApp        *adminapp.App
	routePath       string
	dataGridHandler *datagrid.Handler[models.Admin]
	adminRepo       adminRepo
	authRuntime     adminAuthRuntime
	fileRepo        fileRepo
	previewService  previewService
	auditWriter     adminactionauditrepo.Writer
}

// NewHandler создает новый обработчик администраторов.
func NewHandler(
	adminApp *adminapp.App,
	adminRepo adminRepo,
	authRuntime adminAuthRuntime,
	fileRepo fileRepo,
	previewService previewService,
	auditWriter adminactionauditrepo.Writer,
) *Handler {
	routePath := "/admins"
	config := datagrid.FilterConfig[models.Admin]{
		RoutePath:        routePath,
		Entity:           string(uploadhost.ObjectTypeAdmin),
		Title:            "Администраторы",
		CreateButtonText: "Добавить администратора",
		EmptyMessage:     "Администраторы не найдены",
		Repository:       adminRepo,
		Creatable:        true,
		Refreshable:      true,
		Columns: []datagrid.Column{
			{
				Key:        "id",
				Label:      "ID",
				Type:       "number",
				Sortable:   true,
				Filterable: false,
			},
			{
				Key:        "status",
				Label:      "Статус",
				Type:       "select",
				Sortable:   false,
				Filterable: true,
				FilterOptions: []map[string]string{
					{"value": "all", "label": "Все"},           //nolint:goconst // required
					{"value": "active", "label": "Активные"},   //nolint:goconst // required
					{"value": "deleted", "label": "Удалённые"}, //nolint:goconst // required
				},
				Badges: map[string]map[string]string{
					"active":  {"label": "Активный", "variant": "success"}, //nolint:goconst // required
					"deleted": {"label": "Удалён", "variant": "danger"},
				},
			},
			{
				Key:        "emailStatus",
				Label:      "Email статус",
				Type:       "select",
				Sortable:   false,
				Filterable: true,
				FilterOptions: []map[string]string{
					{"value": "any", "label": "Любой"},
					{"value": "confirmed", "label": "Подтверждён"},      //nolint:goconst // required
					{"value": "unconfirmed", "label": "Не подтверждён"}, //nolint:goconst // required
				},
				Badges: map[string]map[string]string{
					"confirmed":   {"label": "Подтверждён", "variant": "success"},
					"unconfirmed": {"label": "Не подтверждён", "variant": "warning"},
				},
			},
			{
				Key:               "name", //nolint:goconst // required
				Label:             "Имя",
				Type:              "text", //nolint:goconst // required
				Sortable:          true,
				Filterable:        true,
				FilterPlaceholder: "Введите имя",
			},
			{
				Key:               "lastName", //nolint:goconst // required
				Label:             "Фамилия",
				Type:              "text",
				Sortable:          true,
				Filterable:        true,
				FilterPlaceholder: "Введите фамилию",
			},
			{
				Key:               "email", //nolint:goconst // required
				Label:             "Email", //nolint:goconst // required
				Type:              "text",
				Sortable:          true,
				Filterable:        true,
				FilterPlaceholder: "Введите email",
			},
			{
				Key:        "rolesSummary",
				Label:      "Роли",
				Type:       "text",
				Sortable:   false,
				Filterable: false,
			},
			{
				Key:        "lastLoginAt",
				Label:      "Последний вход",
				Type:       "date",             //nolint:goconst // required
				Format:     "2006-01-02 15:04", //nolint:goconst // required
				Sortable:   false,
				Filterable: false,
			},
			{
				Key:        "emailConfirmedAt",
				Label:      "Email подтверждён",
				Type:       "date",
				Format:     "2006-01-02 15:04",
				Sortable:   false,
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
		Actions: []datagrid.Action[models.Admin]{
			{
				Key:     "view",
				Label:   "Просмотр",
				Icon:    "eye",
				Variant: "secondary",
			},
			{
				Key:     "edit",
				Label:   "Редактировать",
				Icon:    "edit",
				Variant: "primary",
			},
			{
				Key:     "delete",
				Label:   "Удалить",
				Icon:    "trash",
				Variant: "danger",
			},
		},
		RowDecorators: []datagrid.RowDecorator[models.Admin]{
			datagrid.NewRowDecorator(
				func(ctx context.Context, items []models.Admin) error {
					if lister, ok := adminRepo.(adminRoleNamesLister); ok {
						adminIDs := make([]int64, 0, len(items))
						for _, item := range items {
							adminIDs = append(adminIDs, item.ID)
						}
						roleNamesByAdmin, err := lister.ListRoleNamesByAdminIDs(ctx, adminIDs)
						if err != nil {
							return fmt.Errorf("list admin role names: %w", err)
						}
						for index := range items {
							data := items[index].GetData()
							data.Roles = roleNamesByAdmin[items[index].ID]
							items[index].Data = data
						}
						return nil
					}

					for index := range items {
						roles, err := adminApp.RolesService().GetRolesAdmin(ctx, items[index].ID)
						if err != nil {
							return fmt.Errorf("list roles for admin %d: %w", items[index].ID, err)
						}

						roleNames := make([]string, 0, len(roles))
						for _, role := range roles {
							roleNames = append(roleNames, role.Name)
						}
						data := items[index].GetData()
						data.Roles = roleNames
						items[index].Data = data
					}

					return nil
				},
				func(_ context.Context, admin models.Admin) (map[string]any, error) {
					status := "active"
					if admin.DeletedAt.Valid {
						status = "deleted"
					}
					emailStatus := "unconfirmed"
					if admin.GetEmailConfirmedAt() != nil {
						emailStatus = "confirmed"
					}

					rolesSummary := "—"
					if roles := admin.GetRoles(); len(roles) > 0 {
						rolesSummary = strings.Join(roles, ", ")
					}

					computed := map[string]any{
						"status":       status,
						"emailStatus":  emailStatus,
						"rolesSummary": rolesSummary,
					}
					if lastLoginAt := admin.GetLastLoginAt(); lastLoginAt != nil {
						computed["lastLoginAt"] = lastLoginAt.Format(time.RFC3339)
					}
					if confirmedAt := admin.GetEmailConfirmedAt(); confirmedAt != nil {
						computed["emailConfirmedAt"] = confirmedAt.Format(time.RFC3339)
					}

					return computed, nil
				},
			),
		},
		PageComponent: func(c fiber.Ctx, data datagrid.Response[models.Admin]) error {
			dataMap := data.ToAPIResponse()

			return adminApp.HTTPManager().Render(c, "admin/IndexPage", map[string]any{
				"title": data.Title, //nolint:goconst // required
				"data":  dataMap,    //nolint:goconst // required
			})
		},
	}

	return &Handler{
		adminApp:        adminApp,
		routePath:       routePath,
		adminRepo:       adminRepo,
		authRuntime:     authRuntime,
		fileRepo:        fileRepo,
		previewService:  previewService,
		auditWriter:     auditWriter,
		dataGridHandler: datagrid.NewHandler(config, adminApp.Logger()),
	}
}

// RegisterGroupRoutes регистрирует маршруты для администраторов.
func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	readRoutes := route.Group(h.routePath, h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead)) //nolint:lll // required
	h.dataGridHandler.RegisterRoutes(readRoutes, "")
	readRoutes.Post("refresh", h.Refresh)
	readRoutes.Get("create", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate), h.Create) //nolint:lll // required
	readRoutes.Post("create", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionCreate), h.Store) //nolint:lll // required
	readRoutes.Get(":id/edit", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate), h.Edit) //nolint:lll // required
	readRoutes.Get(":id", h.View)
	readRoutes.Put(":id", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionUpdate), h.Update)    //nolint:lll // required
	readRoutes.Delete(":id", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionDelete), h.Delete) //nolint:lll // required
}

func (h *Handler) Create(c fiber.Ctx) error {
	return h.adminApp.HTTPManager().Render(c, "admin/CreatePage", map[string]any{
		"title":      "Создание администратора",
		"entityType": uploadhost.ObjectTypeAdmin, //nolint:goconst // required
	})
}

func (h *Handler) Edit(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("admin handler edit: %w", err)
	}

	admin, err := h.adminRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("admin handler edit: error: %w", err)
	}
	if admin.ID <= 0 {
		return fiber.NewError(fiber.StatusNotFound, "Администратор не найден")
	}

	filePreview, err := h.loadPreview(c, admin.GetPreviewFileID())
	if err != nil {
		return fmt.Errorf("load admin preview: %w", err)
	}

	return h.adminApp.HTTPManager().Render(c, "admin/EditPage", map[string]any{
		"title":         "Редактирование администратора",
		"data":          admin,
		"admin_preview": filePreview,
		"entityType":    uploadhost.ObjectTypeAdmin,
	})
}

func (h *Handler) View(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("admin handler view: %w", err)
	}

	admin, err := h.adminRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("admin handler view: error: %w", err)
	}
	if admin.ID <= 0 {
		return fiber.NewError(fiber.StatusNotFound, "Администратор не найден")
	}

	filePreview, err := h.loadPreview(c, admin.GetPreviewFileID())
	if err != nil {
		return fmt.Errorf("load admin preview: %w", err)
	}

	return h.adminApp.HTTPManager().Render(c, "admin/ViewPage", map[string]any{
		"title":         "Просмотр администратора",
		"data":          admin,
		"admin_preview": filePreview,
		"entityType":    uploadhost.ObjectTypeAdmin,
	})
}

func (h *Handler) Store(c fiber.Ctx) error {
	type input struct {
		Username        string `json:"username"`
		Name            string `json:"name"`
		LastName        string `json:"lastName"`
		Email           string `json:"email"`
		Password        string `json:"password"`
		PasswordConfirm string `json:"passwordConfirm"`
		PreviewID       *int64 `json:"previewId"`
	}
	var form input
	if err := c.Bind().Body(&form); err != nil {
		return fmt.Errorf("admin handler store parse: %w", err)
	}
	h.adminApp.HTTPManager().WithFlashOld(c, map[string]any{
		"username":  form.Username,
		"name":      form.Name,
		"lastName":  form.LastName,
		"email":     form.Email,
		"previewId": form.PreviewID, //nolint:goconst // required
	})

	validationErrs := formvalidator.Validate(
		&validators.StringIsPresent{Name: "Name", Field: form.Name, Message: "Name не задан"},
		&validators.EmailIsPresent{Name: "Email", Field: form.Email, Message: "Email не задан или неверный"},
		&validators.StringsMatch{
			Name:  "Password",
			Field: form.Password, Field2: form.PasswordConfirm,
			Message: "Password не задан или неверный",
		},
	)
	if validationErrs.HasAny() {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Форма заполнена не верно!")
		return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, validationErrs.Errors)
	}

	previewFile, previewValidationErrors, err := h.fetchPreviewFile(c, form.PreviewID, 0)
	if err != nil {
		return fmt.Errorf("admin handler store preview validation: %w", err)
	}
	if len(previewValidationErrors) > 0 {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Форма заполнена не верно!")
		return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, previewValidationErrors)
	}

	if h.authRuntime == nil {
		h.adminApp.Logger().ErrorContext(c, "admin auth Runtime unavailable")
		h.adminApp.HTTPManager().WithFlashWarning(c, "Что-то пошло не так, запись не создана!")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	prepared, err := h.prepareAdminProvision(c, goauth.RegisterRequest{
		Email: form.Email, Password: form.Password,
		Profile: goauth.BasicProfile{
			Username: form.Username, DisplayName: strings.TrimSpace(form.Name + " " + form.LastName),
			GivenName: form.Name, FamilyName: form.LastName,
		},
	})
	if err != nil {
		return h.storeFailure(c, err)
	}
	actor := shared.MustGetAdminAuth(c)
	requestID := requestid.FromContext(c)
	var created models.Admin
	err = h.adminApp.InTransaction(c, func(ctx context.Context) error {
		account, err := prepared(ctx)
		if err != nil {
			return err
		}
		created, err = h.adminRepo.ProvisionAccount(ctx, account, identity.NewUserID())
		if err != nil {
			return err
		}
		if previewFile != nil {
			if _, err = h.preparePreview(ctx, &created, previewFile); err != nil {
				return err
			}
			if err = h.adminRepo.UpdateAdmin(ctx, created.ID, created); err != nil {
				return err
			}
		}
		if h.auditWriter == nil {
			return errors.New("admin audit writer is required")
		}
		return h.auditWriter.RecordAdminAction(ctx, adminactionauditrepo.Record{
			ActorAdminID: actor.ID, ActorSubjectID: actor.AuthSubjectID().String(), Action: "admin.created",
			TargetType: string(uploadhost.ObjectTypeAdmin), TargetID: strconv.FormatInt(created.ID, 10), RequestID: requestID,
		})
	})
	if err != nil {
		return h.storeFailure(c, err)
	}
	id := created.ID

	redirectPath := h.routePath + "/" + strconv.FormatInt(id, 10)
	h.adminApp.HTTPManager().WithFlashSuccess(c, "Запись успешна создана!")
	return h.adminApp.HTTPManager().Redirect(c, redirectPath)
}

func (h *Handler) Update(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("admin handler update: %w", err)
	}

	var form adminUpdateInput
	if err := c.Bind().Body(&form); err != nil {
		return fmt.Errorf("admin handler update parse: %w", err)
	}
	h.adminApp.HTTPManager().WithFlashOld(c, map[string]any{
		"username":  form.Username,
		"name":      form.Name,
		"lastName":  form.LastName,
		"email":     form.Email,
		"previewId": form.PreviewID,
	})

	validationErrs := formvalidator.Validate(
		&validators.StringIsPresent{Name: "Name", Field: form.Name, Message: "Name не задан"},
		&validators.EmailIsPresent{Name: "Email", Field: form.Email, Message: "Email не задан или неверный"},
	)
	if validationErrs.HasAny() {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Форма заполнена не верно!")
		return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, validationErrs.Errors)
	}

	previewFile, previewValidationErrors, err := h.fetchPreviewFile(c, form.PreviewID, id)
	if err != nil {
		return fmt.Errorf("admin handler update preview validation: %w", err)
	}
	if len(previewValidationErrors) > 0 {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Форма заполнена не верно!")
		return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, previewValidationErrors)
	}

	actor := shared.MustGetAdminAuth(c)
	requestID := requestid.FromContext(c)
	err = h.adminApp.InTransaction(c, func(ctx context.Context) error {
		return h.updateAdmin(ctx, id, form, previewFile, *actor, requestID)
	})
	if err != nil {
		var httpErr *fiber.Error
		if errors.As(err, &httpErr) && httpErr.Code == fiber.StatusNotFound {
			return httpErr
		}
		h.adminApp.Logger().ErrorContext(c, "admin repo update in http handler", logger.Error(err))
		if errors.Is(err, adminrepo.ErrAdminVersionConflict) || errors.Is(err, uploadintegration.ErrPreviewBindingConflict) {
			h.adminApp.HTTPManager().WithFlashWarning(c, "Данные администратора изменились. Обновите страницу и повторите действие.")
			return h.adminApp.HTTPManager().RedirectBack(c)
		}
		message := "Не удалось обновить администратора."
		if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
			message = "Результат операции неизвестен. Обновите страницу перед повтором."
		}
		h.adminApp.HTTPManager().WithFlashError(c, message)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Запись успешна обновлена!")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func adminSubjectID(admin models.Admin) integrationroles.SubjectID {
	return admin.AuthSubjectID()
}

func (h *Handler) Delete(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return fmt.Errorf("admin handler view: %w", err)
	}

	adminAuth := shared.MustGetAdminAuth(c)
	if adminAuth.ID == id {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Нельзя удалить самого себя!")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	target, err := h.adminRepo.GetByID(c, id)
	if err != nil {
		return fmt.Errorf("load admin before delete: %w", err)
	}
	if target.ID <= 0 {
		return fiber.NewError(fiber.StatusNotFound, "Администратор не найден")
	}
	roles, err := h.adminApp.RolesService().GetRolesAdmin(c, id)
	if err != nil {
		return fmt.Errorf("load admin roles before delete: %w", err)
	}
	for _, role := range roles {
		if role.Slug == integrationroles.SuperAdminRole {
			return fiber.NewError(fiber.StatusForbidden, "Нельзя удалить Super Admin")
		}
	}

	if h.auditWriter == nil {
		return errors.New("admin action audit writer is required")
	}
	err = h.adminApp.InTransaction(c, func(ctx context.Context) error {
		if err := h.adminRepo.SoftDeleteByID(ctx, id); err != nil {
			return err
		}
		return h.auditWriter.RecordAdminAction(ctx, adminactionauditrepo.Record{
			ActorAdminID:   adminAuth.ID,
			ActorSubjectID: adminAuth.AuthSubjectID().String(),
			Action:         adminactionauditrepo.ActionAdminSoftDeleted,
			TargetType:     string(uploadhost.ObjectTypeAdmin),
			TargetID:       strconv.FormatInt(id, 10),
			RequestID:      requestid.FromContext(c),
		})
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "Администратор не найден")
		}
		h.adminApp.Logger().ErrorContext(c, "admin repo delete in http handler", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Что-то пошло не так, запись не удалена!")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Запись успешно удалена!")

	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) Refresh(c fiber.Ctx) error {
	h.adminApp.HTTPManager().WithFlashInfo(c, "Данные обновлены!")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) loadPreview(ctx context.Context, id int64) (*uploadhost.File, error) {
	if h.previewService == nil {
		return nil, nil //nolint:nilnil // uploads-disabled rendering intentionally has no preview
	}
	return h.previewService.LoadPreview(ctx, id)
}

func (h *Handler) fetchPreviewFile(
	ctx context.Context,
	previewID *int64,
	adminID int64,
) (*uploadhost.File, goinertia.ValidationErrors, error) {
	if previewID == nil || *previewID <= 0 {
		return nil, nil, nil
	}

	if h.fileRepo == nil {
		return nil, goinertia.ValidationErrors{"previewId": {"Загрузка файлов отключена"}}, nil
	}
	file, err := h.fileRepo.GetByID(ctx, *previewID)
	if err != nil {
		return nil, nil, fmt.Errorf("file repo get by id: %w", err)
	}

	if file.ID == 0 {
		return nil, goinertia.ValidationErrors{
			"previewId": {"Файл превью не найден"},
		}, nil
	}

	if !uploadintegration.Finalized(file) || file.ObjectType != uploadhost.ObjectTypeAdmin || !isPreviewImage(file.MimeType) {
		return nil, goinertia.ValidationErrors{
			"previewId": {"Недопустимый файл для превью"},
		}, nil
	}
	if file.ObjectID != nil {
		if adminID <= 0 || file.ObjectID.Int64() != adminID {
			return nil, goinertia.ValidationErrors{"previewId": {"Файл принадлежит другому администратору"}}, nil
		}
	} else {
		actor := shared.GetAdminAuth(ctx)
		if actor == nil || file.ManagerID == nil || *file.ManagerID != actor.ID {
			return nil, goinertia.ValidationErrors{"previewId": {"Файл загружен другим администратором"}}, nil
		}
	}

	return &file, nil, nil
}

func isPreviewImage(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (h *Handler) preparePreview(ctx context.Context, admin *models.Admin, preview *uploadhost.File) (bool, error) {
	if preview == nil {
		if admin.Data == nil || admin.Data.PreviewFileID == nil {
			return false, nil
		}

		admin.Data.PreviewFileID = nil
		return true, nil
	}

	if admin.Data == nil {
		admin.Data = &models.AdminData{}
	}

	needsFileUpdate := preview.ObjectType != uploadhost.ObjectTypeAdmin ||
		preview.ObjectID == nil ||
		(preview.ObjectID != nil && preview.ObjectID.Int64() != admin.ID)

	binder, ok := h.fileRepo.(interface {
		BindPreview(ctx context.Context, binding uploadintegration.PreviewBinding) error
	})
	if !ok || preview.ManagerID == nil {
		return false, errors.New("preview repository requires atomic BindPreview")
	}
	if err := binder.BindPreview(ctx, uploadintegration.PreviewBinding{
		FileID: preview.ID, ManagerID: *preview.ManagerID, ExpectedObjectType: preview.ObjectType,
		ExpectedObjectID: preview.ObjectID, AdminID: admin.ID,
	}); err != nil {
		return false, err
	}

	if admin.Data.PreviewFileID != nil && *admin.Data.PreviewFileID == preview.ID && !needsFileUpdate {
		return false, nil
	}

	admin.Data.PreviewFileID = pointer.To(preview.ID)

	return true, nil
}

func (h *Handler) getID(c fiber.Ctx) (int64, error) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id is not valid: %v", c.Params("id"))
	}

	return int64(id), nil
}

type adminUpdateInput struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	PreviewID *int64 `json:"previewId"`
}

func (h *Handler) updateAdmin(ctx context.Context, id int64, form adminUpdateInput,
	previewFile *uploadhost.File, actor models.SessionAdmin, requestID string,
) error {
	currentAdmin, err := h.adminRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("admin repo found ID: %w", err)
	}
	if currentAdmin.ID <= 0 {
		return fiber.NewError(fiber.StatusNotFound, "Администратор не найден")
	}
	currentEmail := currentAdmin.Email

	currentAdmin.Username = form.Username
	currentAdmin.Name = form.Name
	currentAdmin.LastName = form.LastName
	currentAdmin.UpdatedAt = time.Now()

	if h.previewService != nil {
		if _, err := h.preparePreview(ctx, &currentAdmin, previewFile); err != nil {
			return fmt.Errorf("admin prepare preview: %w", err)
		}
	}

	subjectID := adminSubjectID(currentAdmin)
	if _, err := h.authRuntime.UpdateBasicProfile(ctx, subjectID, goauth.BasicProfile{
		Username:    currentAdmin.Username,
		DisplayName: strings.TrimSpace(currentAdmin.Name + " " + currentAdmin.LastName),
		GivenName:   currentAdmin.Name, FamilyName: currentAdmin.LastName,
	}); err != nil {
		return fmt.Errorf("admin canonical profile update: %w", err)
	}

	if err := h.adminRepo.UpdateAdmin(ctx, id, currentAdmin); err != nil {
		return fmt.Errorf("admin repo update ID: %w", err)
	}
	if form.Email != "" && form.Email != currentEmail {
		if err := h.authRuntime.RequestEmailChange(ctx, subjectID, form.Email); err != nil {
			return fmt.Errorf("admin canonical email change request: %w", err)
		}
	}

	if h.auditWriter == nil {
		return errors.New("admin audit writer is required")
	}
	return h.auditWriter.RecordAdminAction(ctx, adminactionauditrepo.Record{
		ActorAdminID: actor.ID, ActorSubjectID: actor.AuthSubjectID().String(), Action: "admin.updated",
		TargetType: string(uploadhost.ObjectTypeAdmin), TargetID: strconv.FormatInt(id, 10), RequestID: requestID,
	})
}

func (h *Handler) storeFailure(c fiber.Ctx, err error) error {
	if errors.Is(err, goauth.ErrInvalidPassword) || errors.Is(err, goauth.ErrCommonPassword) ||
		errors.Is(err, goauth.ErrInvalidCredentials) || errors.Is(err, goauth.ErrIdentifierAlreadyExists) {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Пароль или email не соответствуют требованиям")
		return h.adminApp.HTTPManager().RedirectBackWithValidationErrors(c, goinertia.ValidationErrors{
			"password": {"Пароль или email не соответствуют требованиям"},
		})
	}
	h.adminApp.Logger().ErrorContext(c, "admin provision command", logger.Error(err))
	message := "Не удалось создать администратора."
	if errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
		message = "Результат операции неизвестен. Обновите список перед повтором."
	}
	h.adminApp.HTTPManager().WithFlashError(c, message)
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) prepareAdminProvision(ctx context.Context, request goauth.RegisterRequest) (
	func(context.Context) (goauth.Account, error), error,
) {
	if preparer, ok := h.authRuntime.(interface {
		PrepareTrustedAdmin(ctx context.Context, request goauth.RegisterRequest) (func(context.Context) (goauth.Account, error), error)
	}); ok {
		return preparer.PrepareTrustedAdmin(ctx, request)
	}
	return func(ctx context.Context) (goauth.Account, error) {
		return h.authRuntime.ProvisionTrustedAdmin(ctx, request)
	}, nil
}
