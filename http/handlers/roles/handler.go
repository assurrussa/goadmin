package roles

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

//go:generate toolsmocks

const (
	adminListPageSize = 200
	gridBadgeLabelKey = "label"

	superAdminChangeForbiddenMessage = "Недостаточно прав для изменения супер-администратора"
	superRoleChangeForbiddenMessage  = "Недостаточно прав для изменения роли супер-администратора"
)

type listRolesUseCase interface {
	Handle(ctx context.Context, req integrationroles.ListRolesRequest) (integrationroles.ListRolesResponse, error)
}

type getRoleUseCase interface {
	Handle(ctx context.Context, req integrationroles.GetRoleRequest) (integrationroles.GetRoleResponse, error)
}

type createRoleUseCase interface {
	Handle(ctx context.Context, req integrationroles.CreateRoleRequest) (integrationroles.CreateRoleResponse, error)
}

type updateRoleUseCase interface {
	Handle(ctx context.Context, req integrationroles.UpdateRoleRequest) (integrationroles.UpdateRoleResponse, error)
}

type deleteRoleUseCase interface {
	Handle(ctx context.Context, req integrationroles.DeleteRoleRequest) (integrationroles.DeleteRoleResponse, error)
}

type setPermissionsUseCase interface {
	Handle(ctx context.Context, req integrationroles.SetRolePermissionsRequest) (integrationroles.SetRolePermissionsResponse, error)
}

type AssignAdminRolesRequest struct {
	AdminID int64
	RoleIDs []int64
}

type AssignAdminRolesResponse struct {
	Assigned int `json:"assigned"`
}

type assignAdminRolesUseCase interface {
	Handle(ctx context.Context, req AssignAdminRolesRequest) (AssignAdminRolesResponse, error)
}

type listPermissionsUseCase interface {
	Handle(ctx context.Context, req integrationroles.ListPermissionsRequest) (integrationroles.ListPermissionsResponse, error)
}

type listRolePermissionsUseCase interface {
	Handle(ctx context.Context, req integrationroles.ListRolePermissionsRequest) (integrationroles.ListRolePermissionsResponse, error) //nolint:lll // required
}

type ListAdminRolesRequest struct {
	AdminID int64
}

type ListAdminRolesResponse struct {
	Roles []integrationroles.Role `json:"roles"`
}

type listAdminRolesUseCase interface {
	Handle(ctx context.Context, req ListAdminRolesRequest) (ListAdminRolesResponse, error)
}

type listAllRolesUseCase interface {
	Handle(ctx context.Context, req integrationroles.ListAllRolesRequest) (integrationroles.ListAllRolesResponse, error)
}

type adminRepository interface {
	GetByID(ctx context.Context, id int64) (models.Admin, error)
	GetBySubjectID(ctx context.Context, subjectID integrationroles.SubjectID) (models.Admin, error)
	GetByUUID(ctx context.Context, id identity.UserID) (models.Admin, error)
	GetList(ctx context.Context, filters datagrid.Filtered) ([]models.Admin, int, error)
}

// UseCases агрегирует зависимости хендлера.
type UseCases struct {
	ListRoles           listRolesUseCase
	GetRole             getRoleUseCase
	CreateRole          createRoleUseCase
	UpdateRole          updateRoleUseCase
	DeleteRole          deleteRoleUseCase
	SetPermissions      setPermissionsUseCase
	AssignAdminRoles    assignAdminRolesUseCase
	ListPermissions     listPermissionsUseCase
	ListRolePermissions listRolePermissionsUseCase
	ListAdminRoles      listAdminRolesUseCase
	ListAllRoles        listAllRolesUseCase
}

type Handler struct {
	adminApp  *adminapp.App
	routePath string
	logger    logger.Logger

	dataGridHandler *datagrid.Handler[roleGridItem]

	adminRepo adminRepository

	listRoles           listRolesUseCase
	getRole             getRoleUseCase
	createRole          createRoleUseCase
	updateRole          updateRoleUseCase
	deleteRole          deleteRoleUseCase
	setPermissions      setPermissionsUseCase
	assignAdminRoles    assignAdminRolesUseCase
	listPermissions     listPermissionsUseCase
	listRolePermissions listRolePermissionsUseCase
	listAdminRoles      listAdminRolesUseCase
	listAllRoles        listAllRolesUseCase
}

func NewHandler(
	adminApp *adminapp.App,
	adminRepo adminRepository,
	useCases UseCases,
) *Handler {
	routePath := "/roles"
	updatePermissionKey := integrationroles.NewPermissionKey(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate) //nolint:lll // required

	gridHandler := datagrid.NewHandler(datagrid.FilterConfig[roleGridItem]{
		RoutePath:         routePath,
		Entity:            integrationroles.PermissionDomainRoles.String(),
		Title:             "Роли доступа",
		Repository:        newRolesGridRepository(useCases.ListRoles),
		DefaultSort:       "id",
		DefaultOrder:      "desc",
		PageSize:          20,
		Creatable:         true,
		Refreshable:       true,
		SearchPlaceholder: "Поиск по названию или slug",
		CreateButtonText:  "Добавить роль",
		EmptyMessage:      "Роли отсутствуют",
		IDKey:             "id",
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Type: "number", Sortable: true},
			{Key: "name", Label: "Название", Type: "text", Sortable: true, Filterable: true, FilterPlaceholder: "Введите название"}, //nolint:goconst,lll // required
			{Key: "slug", Label: "Slug", Type: "text", Sortable: true, Filterable: true, FilterPlaceholder: "Введите slug"},         //nolint:goconst,lll // required
			{Key: "scope", Label: "Область доступа", Type: "text"},
			{
				Key:        "isSystem", //nolint:goconst // required
				Label:      "Тип",
				Type:       "select",
				Filterable: true,
				Badges: map[string]map[string]string{
					"true":  {gridBadgeLabelKey: "Системная", "variant": "warning"},
					"false": {gridBadgeLabelKey: "Пользовательская", "variant": "success"},
				},
				FilterOptions: []map[string]string{
					{"value": "", "label": "Все"}, //nolint:goconst // required
					{"value": "system", "label": "Системные"},
					{"value": "regular", "label": "Пользовательские"},
				},
			},
		},
		Actions: []datagrid.Action[roleGridItem]{
			{Key: "view", Label: "Просмотр", Icon: "eye", Variant: "secondary"},
			{
				Key:     "edit",
				Label:   "Редактировать",
				Icon:    "edit",
				Variant: "primary",
				CanView: func(ctx context.Context, item roleGridItem) bool {
					admin := adminmiddleware.MustGetAdminAuth(ctx)

					if !adminApp.RolesService().AdminGuardCheck(ctx, admin.ID, updatePermissionKey) {
						return false
					}

					if err := checkSuperRole(ctx, adminApp, item.Slug, admin.ID); err != nil {
						return false
					}

					return true
				},
			},
			{
				Key:     "delete",
				Label:   "Удалить",
				Icon:    "trash",
				Variant: "danger",
				CanView: func(ctx context.Context, item roleGridItem) bool {
					if item.IsSystem {
						return false
					}

					admin := adminmiddleware.MustGetAdminAuth(ctx)
					if err := checkSuperRole(ctx, adminApp, item.Slug, admin.ID); err != nil {
						return false
					}

					return true
				},
			},
		},
		PageComponent: func(c fiber.Ctx, data datagrid.Response[roleGridItem]) error {
			return adminApp.HTTPManager().Render(c, "roles/IndexPage", map[string]any{
				"title": data.Title, //nolint:goconst // required
				"data":  data.ToAPIResponse(),
			})
		},
	}, adminApp.Logger())

	return &Handler{
		adminApp:            adminApp,
		routePath:           routePath,
		logger:              adminApp.Logger().WithNamed("roles-handler"),
		dataGridHandler:     gridHandler,
		adminRepo:           adminRepo,
		listRoles:           useCases.ListRoles,
		getRole:             useCases.GetRole,
		createRole:          useCases.CreateRole,
		updateRole:          useCases.UpdateRole,
		deleteRole:          useCases.DeleteRole,
		setPermissions:      useCases.SetPermissions,
		assignAdminRoles:    useCases.AssignAdminRoles,
		listPermissions:     useCases.ListPermissions,
		listRolePermissions: useCases.ListRolePermissions,
		listAdminRoles:      useCases.ListAdminRoles,
		listAllRoles:        useCases.ListAllRoles,
	}
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	readRoutes := route.Group(h.routePath, h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionRead)) //nolint:lll // required
	h.dataGridHandler.RegisterRoutes(readRoutes, "")
	readRoutes.Post("refresh", h.Refresh)

	readRoutes.Get("create", h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionCreate), h.CreateForm) //nolint:lll // required
	readRoutes.Post("", h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionCreate), h.Store)
	readRoutes.Get("permissions", h.AvailablePermissions)

	readRoutes.Get(":id", h.View)
	readRoutes.Get(":id/edit", h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate), h.EditForm) //nolint:lll // required
	readRoutes.Put(":id", h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate), h.Update)        //nolint:lll // required
	readRoutes.Delete(":id", h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionDelete), h.Delete)     //nolint:lll // required

	readRoutes.Get(":id/permissions", h.Permissions)
	readRoutes.Post(
		":id/permissions",
		h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate),
		h.UpdatePermissions,
	)
	readRoutes.Get(":id/admins", h.RoleAdmins)

	assignRoutes := route.Group(h.routePath, h.adminApp.Guard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionAssign)) //nolint:lll // required
	assignRoutes.Post("assign", h.AssignRoles)
	assignRoutes.Get(":id/admins/search", h.SearchAdmins)
	assignRoutes.Post(":id/admins/attach", h.AttachRoleToAdmin)
	assignRoutes.Post(":id/admins/detach", h.DetachRoleFromAdmin)
}

func (h *Handler) Refresh(c fiber.Ctx) error {
	h.adminApp.HTTPManager().WithFlashInfo(c, "Данные обновлены")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) CreateForm(c fiber.Ctx) error {
	perms, err := h.loadPermissions(c)
	if err != nil {
		h.logger.ErrorContext(c, "load permissions", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить права доступа", err)
	}

	return h.adminApp.HTTPManager().Render(c, "roles/CreatePage", map[string]any{
		"title":       "Создание роли",
		"permissions": perms, //nolint:goconst // required
	})
}

func (h *Handler) Store(c fiber.Ctx) error {
	var payload createRolePayload
	if err := c.Bind().Body(&payload); err != nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Некорректные данные формы")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.rememberCreatePayload(c, payload)

	req := integrationroles.CreateRoleRequest{
		Slug:           strings.TrimSpace(payload.Slug),
		Name:           strings.TrimSpace(payload.Name),
		Description:    payload.Description,
		IsSystem:       payload.IsSystem,
		PermissionKeys: payload.Permissions,
	}

	if _, err := h.createRole.Handle(c, req); err != nil {
		h.flashDomainError(c, err, "Не удалось создать роль")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Роль создана")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) View(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	detail, err := h.fetchRoleDetail(c, roleID, true)
	if err != nil {
		if errors.Is(err, integrationroles.ErrRoleNotFound) {
			return goinertia.NewError(fiber.StatusNotFound, "Роль не найдена", err)
		}
		h.logger.ErrorContext(c, "load role", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить роль", err)
	}

	if assigned, assignErr := h.listRoleAdmins(c, detail.ID); assignErr != nil {
		h.logger.WarnContext(c, "load role admins", logger.Error(assignErr))
	} else {
		detail.Admins = assigned
	}

	return h.adminApp.HTTPManager().Render(c, "roles/ViewPage", map[string]any{
		"title": "Роль " + detail.Name,
		"role":  detail,
	})
}

func (h *Handler) EditForm(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	detail, err := h.fetchRoleDetail(c, roleID, true)
	if err != nil {
		if errors.Is(err, integrationroles.ErrRoleNotFound) {
			return goinertia.NewError(fiber.StatusNotFound, "Роль не найдена", err)
		}
		h.logger.ErrorContext(c, "load role for edit", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить роль", err)
	}

	perms, err := h.loadPermissions(c)
	if err != nil {
		h.logger.ErrorContext(c, "load permissions", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить права доступа", err)
	}

	return h.adminApp.HTTPManager().Render(c, "roles/EditPage", map[string]any{
		"title":       "Редактирование " + detail.Name,
		"role":        detail,
		"permissions": perms,
	})
}

func (h *Handler) checkSuperRole(ctx context.Context, roleSlug string, adminID int64) error {
	return checkSuperRole(ctx, h.adminApp, roleSlug, adminID)
}

func checkSuperRole(
	ctx context.Context,
	adminApp *adminapp.App,
	roleSlug string,
	adminID int64,
) error {
	if roleSlug != integrationroles.SuperAdminRole {
		return nil
	}

	if adminApp.RolesService().IsSuperAdmin(ctx, adminID) {
		return nil
	}

	return fmt.Errorf("forbidden: %s -> super_role", integrationroles.PermissionDomainRoles)
}

func (h *Handler) Update(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	roleDetail, handled, err := h.ensureRoleMutable(c, roleID, "load role for update")
	if handled {
		return err
	}

	var payload updateRolePayload
	if err := c.Bind().Body(&payload); err != nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Некорректные данные формы")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if err := h.checkSuperRole(c, roleDetail.Slug, adminmiddleware.MustGetAdminAuth(c).ID); err != nil {
		return goinertia.NewError(fiber.StatusForbidden, err.Error())
	}

	h.rememberUpdatePayload(c, payload)

	slug := strings.TrimSpace(payload.Slug)
	if roleDetail.Slug == integrationroles.SuperAdminRole {
		h.adminApp.HTTPManager().WithFlashWarning(c, "Нельзя менять slug у супер админа")
		slug = integrationroles.SuperAdminRole
	}

	req := integrationroles.UpdateRoleRequest{
		ID:             roleID,
		Slug:           slug,
		Name:           strings.TrimSpace(payload.Name),
		Description:    payload.Description,
		IsSystem:       payload.IsSystem,
		PermissionKeys: payload.Permissions,
	}

	if _, err := h.updateRole.Handle(c, req); err != nil {
		h.flashDomainError(c, err, "Не удалось обновить роль")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Роль обновлена")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) Delete(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	roleDetail, handled, err := h.ensureRoleMutable(c, roleID, "load role for delete")
	if handled {
		return err
	}

	if err := h.checkSuperRole(c, roleDetail.Slug, adminmiddleware.MustGetAdminAuth(c).ID); err != nil {
		return goinertia.NewError(fiber.StatusForbidden, err.Error())
	}

	if _, err := h.deleteRole.Handle(c, integrationroles.DeleteRoleRequest{RoleID: roleID}); err != nil {
		h.flashDomainError(c, err, "Не удалось удалить роль")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Роль удалена")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) Permissions(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	resp, err := h.listRolePermissions.Handle(c, integrationroles.ListRolePermissionsRequest{RoleID: roleID})
	if err != nil {
		h.logger.ErrorContext(c, "list role permissions", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить права роли", err)
	}

	return c.JSON(rolePermissionsResponse{Permissions: mapRolePermissions(resp.Permissions)})
}

func (h *Handler) UpdatePermissions(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	roleDetail, handled, err := h.ensureRoleMutable(c, roleID, "load role for permissions update")
	if handled {
		return err
	}

	if err := h.checkSuperRole(c, roleDetail.Slug, adminmiddleware.MustGetAdminAuth(c).ID); err != nil {
		return goinertia.NewError(fiber.StatusForbidden, err.Error())
	}

	var payload setPermissionsPayload
	if err := c.Bind().Body(&payload); err != nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Некорректные данные формы")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	req := integrationroles.SetRolePermissionsRequest{
		RoleID:         roleID,
		PermissionKeys: payload.Permissions,
	}
	if _, err := h.setPermissions.Handle(c, req); err != nil {
		h.flashDomainError(c, err, "Не удалось обновить права")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Права обновлены")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) AssignRoles(c fiber.Ctx) error {
	var payload assignRolesPayload
	if err := c.Bind().Body(&payload); err != nil {
		h.adminApp.HTTPManager().WithFlashError(c, "Некорректные данные формы")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashOld(c, map[string]any{
		"adminId": payload.AdminID, //nolint:goconst // required
		"roleIds": payload.RoleIDs,
	})

	targetRoles, err := h.loadAdminRoles(c, payload.AdminID)
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить роли администратора", fiber.StatusInternalServerError)
	}

	targetHasSuper := adminRoleListHasSlug(targetRoles, integrationroles.SuperAdminRole)
	requestIncludesSuper, err := h.hasSuperRoleIDs(c, payload.RoleIDs)
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить роль", fiber.StatusInternalServerError)
	}

	if targetHasSuper || requestIncludesSuper {
		if !h.adminApp.RolesService().IsSuperAdmin(c, adminmiddleware.MustGetAdminAuth(c).ID) {
			if requestIncludesSuper {
				return h.respondSuperRoleForbidden(c)
			}
			return h.respondSuperAdminForbidden(c)
		}
	}

	assignReq := AssignAdminRolesRequest(payload)
	if _, err := h.assignAdminRoles.Handle(c, assignReq); err != nil {
		h.flashDomainError(c, err, "Не удалось назначить роли")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Роли назначены")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) RoleAdmins(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	detail, err := h.fetchRoleDetail(c, roleID, false)
	if err != nil {
		if errors.Is(err, integrationroles.ErrRoleNotFound) {
			return goinertia.NewError(fiber.StatusNotFound, "Роль не найдена", err)
		}
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить роль", err)
	}

	admins, err := h.listRoleAdmins(c, detail.ID)
	if err != nil {
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить администраторов", err)
	}

	return c.JSON(fiber.Map{
		"admins": admins,
	})
}

func (h *Handler) SearchAdmins(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	detail, err := h.fetchRoleDetail(c, roleID, false)
	if err != nil {
		if errors.Is(err, integrationroles.ErrRoleNotFound) {
			return goinertia.NewError(fiber.StatusNotFound, "Роль не найдена", err)
		}
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить роль", err)
	}

	query := strings.TrimSpace(c.Query("search"))
	limit := adminListPageSize

	filters := datagrid.Filters{
		Page:   1,
		Limit:  limit,
		Search: query,
		Fields: map[string]any{},
	}

	admins, _, err := h.adminRepo.GetList(c, filters)
	if err != nil {
		h.logger.ErrorContext(c, "search admins for role", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить администраторов", err)
	}

	options := make([]adminSearchOption, 0, len(admins))
	for _, admin := range admins {
		adminRoles, err := h.loadAdminRoles(c, admin.ID)
		if err != nil {
			h.logger.WarnContext(c, "load admin roles for search", logger.Error(err))
			continue
		}
		option := adminSearchOption{
			ID:       admin.ID,
			Name:     admin.Name,
			LastName: admin.LastName,
			Email:    admin.Email,
			Username: admin.Username,
			Roles:    extractRoleNames(adminRoles),
			HasRole:  adminRoleListHasSlug(adminRoles, detail.Slug),
		}
		options = append(options, option)
	}

	return c.JSON(fiber.Map{"admins": options})
}

func (h *Handler) AttachRoleToAdmin(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	var payload struct {
		AdminID int64 `json:"adminId"`
	}
	if err := c.Bind().Body(&payload); err != nil {
		return h.respondAdminFormError(c, "Некорректные данные формы", fiber.StatusBadRequest, err)
	}
	if payload.AdminID <= 0 {
		return h.respondAdminFormError(c, "Некорректный идентификатор администратора", fiber.StatusBadRequest, nil)
	}

	roleDetail, err := h.fetchRoleDetail(c, roleID, false)
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить роль", fiber.StatusInternalServerError)
	}

	resp, err := h.listAdminRoles.Handle(c, ListAdminRolesRequest{AdminID: payload.AdminID})
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить роли администратора", fiber.StatusInternalServerError)
	}

	targetHasSuper := adminRoleListHasSlug(resp.Roles, integrationroles.SuperAdminRole)
	roleIsSuper := roleDetail.Slug == integrationroles.SuperAdminRole
	if (targetHasSuper || roleIsSuper) &&
		!h.adminApp.RolesService().IsSuperAdmin(c, adminmiddleware.MustGetAdminAuth(c).ID) {
		if roleIsSuper {
			return h.respondSuperRoleForbidden(c)
		}
		return h.respondSuperAdminForbidden(c)
	}

	roleIDs := uniqueRoleIDs(resp.Roles)
	if containsRoleID(roleIDs, roleID) {
		adminModel, getErr := h.adminRepo.GetByID(c, payload.AdminID)
		if getErr != nil {
			return h.respondAdminDomainError(c, getErr, "Не удалось загрузить администратора", fiber.StatusInternalServerError)
		}
		if isInertiaRequest(c) {
			h.adminApp.HTTPManager().WithFlashInfo(c, "Эта роль уже назначена выбранному администратору")
			return h.adminApp.HTTPManager().RedirectBack(c)
		}

		return c.JSON(fiber.Map{"admin": toRoleAssignedAdmin(adminModel)})
	}
	roleIDs = append(roleIDs, roleID)

	if _, err := h.assignAdminRoles.Handle(c, AssignAdminRolesRequest{
		AdminID: payload.AdminID,
		RoleIDs: roleIDs,
	}); err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось назначить роль администратору", fiber.StatusInternalServerError)
	}

	adminModel, err := h.adminRepo.GetByID(c, payload.AdminID)
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить администратора", fiber.StatusInternalServerError)
	}

	return h.respondAttachSuccess(c, adminModel)
}

func (h *Handler) DetachRoleFromAdmin(c fiber.Ctx) error {
	roleID, err := h.parseIDParam(c)
	if err != nil {
		return goinertia.NewError(fiber.StatusBadRequest, err.Error())
	}

	var payload struct {
		AdminID int64 `json:"adminId"`
	}
	if err := c.Bind().Body(&payload); err != nil {
		return h.respondAdminFormError(c, "Некорректные данные формы", fiber.StatusBadRequest, err)
	}
	if payload.AdminID <= 0 {
		return h.respondAdminFormError(c, "Некорректный идентификатор администратора", fiber.StatusBadRequest, nil)
	}

	adminAuth := adminmiddleware.MustGetAdminAuth(c)

	roleDetail, err := h.fetchRoleDetail(c, roleID, false)
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить роль", fiber.StatusInternalServerError)
	}

	resp, err := h.listAdminRoles.Handle(c, ListAdminRolesRequest{AdminID: payload.AdminID})
	if err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось загрузить роли администратора", fiber.StatusInternalServerError)
	}

	targetHasSuper := adminRoleListHasSlug(resp.Roles, integrationroles.SuperAdminRole)
	roleIsSuper := roleDetail.Slug == integrationroles.SuperAdminRole
	if (targetHasSuper || roleIsSuper) &&
		!h.adminApp.RolesService().IsSuperAdmin(c, adminmiddleware.MustGetAdminAuth(c).ID) {
		if roleIsSuper {
			return h.respondSuperRoleForbidden(c)
		}
		return h.respondSuperAdminForbidden(c)
	}

	roleIDs := make([]int64, 0, len(resp.Roles))
	for _, r := range resp.Roles {
		if r.ID != roleID {
			roleIDs = append(roleIDs, r.ID)
			continue
		}

		adopted, err := h.adminApp.RolesService().IsAdoptedDetachRole(c, adminAuth.ID, payload.AdminID, roleID)
		if adopted {
			continue
		}

		message := "Нельзя удалить текущую роль"
		if err != nil {
			message += ":" + err.Error()
		}

		if isInertiaRequest(c) {
			h.withAdminIDError(c, message)
			h.adminApp.HTTPManager().WithFlashError(c, message)
			return h.adminApp.HTTPManager().RedirectBack(c)
		}

		if err != nil {
			return goinertia.NewError(fiber.StatusInternalServerError, message, err)
		}

		return goinertia.NewError(fiber.StatusForbidden, message)
	}

	if len(roleIDs) == len(resp.Roles) {
		if isInertiaRequest(c) {
			h.adminApp.HTTPManager().WithFlashInfo(c, "У администратора нет этой роли")
			return h.adminApp.HTTPManager().RedirectBack(c)
		}
		return c.JSON(fiber.Map{"adminId": payload.AdminID})
	}

	if _, err := h.assignAdminRoles.Handle(c, AssignAdminRolesRequest{
		AdminID: payload.AdminID,
		RoleIDs: roleIDs,
	}); err != nil {
		return h.respondAdminDomainError(c, err, "Не удалось отменить роль у администратора", fiber.StatusInternalServerError)
	}

	return h.respondDetachSuccess(c, payload.AdminID)
}

func (h *Handler) AvailablePermissions(c fiber.Ctx) error {
	resp, err := h.listPermissions.Handle(c, integrationroles.ListPermissionsRequest{})
	if err != nil {
		h.logger.ErrorContext(c, "list permissions", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить права", err)
	}

	return c.JSON(permissionsResponse{Permissions: mapPermissionRecords(resp.Permissions)})
}

func (h *Handler) parseIDParam(c fiber.Ctx) (int64, error) {
	value := c.Params("id")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}

	return id, nil
}

func (h *Handler) fetchRoleDetail(ctx context.Context, roleID int64, withPermissions bool) (roleDetail, error) {
	resp, err := h.getRole.Handle(ctx, integrationroles.GetRoleRequest{RoleID: roleID, IncludePermissions: withPermissions})
	if err != nil {
		return roleDetail{}, err
	}

	return mapRoleDetail(resp), nil
}

func (h *Handler) loadPermissions(ctx context.Context) ([]permissionRecord, error) {
	resp, err := h.listPermissions.Handle(ctx, integrationroles.ListPermissionsRequest{})
	if err != nil {
		return nil, err
	}

	return mapPermissionRecords(resp.Permissions), nil
}

func (h *Handler) ensureRoleMutable(c fiber.Ctx, roleID int64, logContext string) (roleDetail, bool, error) {
	detail, err := h.fetchRoleDetail(c, roleID, false)
	if err != nil {
		if errors.Is(err, integrationroles.ErrRoleNotFound) {
			return roleDetail{}, true, goinertia.NewError(fiber.StatusNotFound, "Роль не найдена", err)
		}
		h.logger.ErrorContext(c, logContext, logger.Error(err))
		return roleDetail{}, true, goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить роль", err)
	}

	return detail, false, nil
}

func (h *Handler) rememberCreatePayload(c fiber.Ctx, payload createRolePayload) {
	h.adminApp.HTTPManager().WithFlashOld(c, map[string]any{
		"slug":        payload.Slug,
		"name":        payload.Name,
		"description": payload.Description,
		"isSystem":    payload.IsSystem,
		"permissions": payload.Permissions,
	})
}

func (h *Handler) rememberUpdatePayload(c fiber.Ctx, payload updateRolePayload) {
	var permissions any
	if payload.Permissions != nil {
		permissions = *payload.Permissions
	}

	h.adminApp.HTTPManager().WithFlashOld(c, map[string]any{
		"slug":        payload.Slug,
		"name":        payload.Name,
		"description": payload.Description,
		"isSystem":    payload.IsSystem,
		"permissions": permissions,
	})
}

func (h *Handler) flashDomainError(c fiber.Ctx, err error, fallback string) {
	message, known := mapRoleDomainError(err, fallback)
	if !known && err != nil {
		h.logger.ErrorContext(c, fallback, logger.Error(err))
	}

	h.adminApp.HTTPManager().WithFlashError(c, message)
}

func (h *Handler) respondSuperAdminForbidden(c fiber.Ctx) error {
	return h.respondAdminFormError(c, superAdminChangeForbiddenMessage, fiber.StatusForbidden, nil)
}

func (h *Handler) respondSuperRoleForbidden(c fiber.Ctx) error {
	return h.respondAdminFormError(c, superRoleChangeForbiddenMessage, fiber.StatusForbidden, nil)
}

type roleGridItem struct {
	ID       int64  `json:"id"`
	UUID     string `json:"uuid"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	IsSystem bool   `json:"isSystem"`
}

func roleScope(slug string, system bool) string {
	switch {
	case slug == integrationroles.SuperAdminRole:
		return "Вся админ-панель"
	case slug == "cms_editor", slug == "cms_publisher", slug == "cms_admin":
		return "Только CMS"
	case slug == "content_admin":
		return "Просмотр ролей и разрешений"
	case system:
		return "Системная область"
	default:
		return "По выбранным разрешениям"
	}
}

type rolesGridRepository struct {
	useCase listRolesUseCase
}

func newRolesGridRepository(useCase listRolesUseCase) *rolesGridRepository {
	return &rolesGridRepository{useCase: useCase}
}

func (r *rolesGridRepository) GetList(ctx context.Context, filtered datagrid.Filtered) ([]roleGridItem, int, error) {
	resp, err := r.useCase.Handle(ctx, integrationroles.ListRolesRequest{IncludeSystem: true})
	if err != nil {
		return nil, 0, err
	}

	items := make([]roleGridItem, 0, len(resp.Roles))
	for _, role := range resp.Roles {
		items = append(items, roleGridItem{
			ID:       role.ID,
			UUID:     role.UUID,
			Slug:     role.Slug,
			Name:     role.Name,
			Scope:    roleScope(role.Slug, role.IsSystem),
			IsSystem: role.IsSystem,
		})
	}

	items = filterRoleGridItems(items, filtered)
	sortRoleGridItems(items, filtered.GetSortBy(), filtered.GetSortOrder())

	total := len(items)
	paged := paginateRoleGridItems(items, filtered.GetOffset(), filtered.GetLimit())

	return paged, total, nil
}

func filterRoleGridItems(items []roleGridItem, filtered datagrid.Filtered) []roleGridItem {
	if len(items) == 0 {
		return items
	}

	fields := filtered.GetFields()
	search := strings.ToLower(strings.TrimSpace(filtered.GetSearch()))
	slugFilter := strings.ToLower(strings.TrimSpace(fieldValue(fields, "slug")))
	nameFilter := strings.ToLower(strings.TrimSpace(fieldValue(fields, "name")))
	typeFilter := strings.ToLower(strings.TrimSpace(fieldValue(fields, "isSystem")))

	if search == "" && slugFilter == "" && nameFilter == "" && typeFilter == "" {
		return items
	}

	result := make([]roleGridItem, 0, len(items))
	for _, item := range items {
		if search != "" &&
			!strings.Contains(strings.ToLower(item.Slug), search) &&
			!strings.Contains(strings.ToLower(item.Name), search) {
			continue
		}
		if slugFilter != "" && !strings.Contains(strings.ToLower(item.Slug), slugFilter) {
			continue
		}
		if nameFilter != "" && !strings.Contains(strings.ToLower(item.Name), nameFilter) {
			continue
		}
		switch typeFilter {
		case "system":
			if !item.IsSystem {
				continue
			}
		case "regular":
			if item.IsSystem {
				continue
			}
		}

		result = append(result, item)
	}

	return result
}

func sortRoleGridItems(items []roleGridItem, sortBy, sortOrder string) {
	if len(items) == 0 {
		return
	}

	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	desc := strings.EqualFold(sortOrder, "desc")

	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return compareRoleGridItems(items[j], items[i], sortBy)
		}
		return compareRoleGridItems(items[i], items[j], sortBy)
	})
}

func compareRoleGridItems(a, b roleGridItem, sortBy string) bool {
	switch sortBy {
	case "name":
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	case "slug":
		return strings.ToLower(a.Slug) < strings.ToLower(b.Slug)
	case "issystem":
		if a.IsSystem == b.IsSystem {
			return a.ID < b.ID
		}
		return a.IsSystem && !b.IsSystem
	default:
		return a.ID < b.ID
	}
}

func paginateRoleGridItems(items []roleGridItem, offset, limit int) []roleGridItem {
	if len(items) == 0 {
		return items
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(items) {
		offset = len(items)
	}

	if limit <= 0 || offset+limit > len(items) {
		limit = len(items) - offset
	}

	sliced := make([]roleGridItem, limit)
	copy(sliced, items[offset:offset+limit])
	return sliced
}

func fieldValue(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}

	if val, ok := fields[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		default:
			return fmt.Sprint(v)
		}
	}

	return ""
}

type roleDetail struct {
	ID          int64               `json:"id"`
	UUID        string              `json:"uuid"`
	Slug        string              `json:"slug"`
	Name        string              `json:"name"`
	Description *string             `json:"description,omitempty"`
	IsSystem    bool                `json:"isSystem"`
	Permissions []permissionDetail  `json:"permissions,omitempty"`
	Admins      []roleAssignedAdmin `json:"admins,omitempty"`
}

type permissionDetail struct {
	Domain      integrationroles.PermissionDomain `json:"domain"`
	Action      integrationroles.PermissionAction `json:"action"`
	Description *string                           `json:"description,omitempty"`
}

type roleAssignedAdmin struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	LastName string `json:"lastName"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type adminSearchOption struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	LastName string   `json:"lastName"`
	Email    string   `json:"email"`
	Username string   `json:"username"`
	Roles    []string `json:"roles,omitempty"`
	HasRole  bool     `json:"hasRole"`
}

type createRolePayload struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	IsSystem    bool     `json:"isSystem"`
	Permissions []string `json:"permissions"`
}

type updateRolePayload struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	IsSystem    *bool     `json:"isSystem"`
	Permissions *[]string `json:"permissions"`
}

type setPermissionsPayload struct {
	Permissions []string `json:"permissions"`
}

type assignRolesPayload struct {
	AdminID int64   `json:"adminId"`
	RoleIDs []int64 `json:"roleIds"`
}

type rolePermissionsResponse struct {
	Permissions []permissionDetail `json:"permissions"`
}

type permissionsResponse struct {
	Permissions []permissionRecord `json:"permissions"`
}

type permissionRecord struct {
	ID          int64                             `json:"id"`
	UUID        string                            `json:"uuid"`
	Domain      integrationroles.PermissionDomain `json:"domain"`
	Action      integrationroles.PermissionAction `json:"action"`
	Description *string                           `json:"description,omitempty"`
}

func mapRoleDetail(resp integrationroles.GetRoleResponse) roleDetail {
	detail := roleDetail{
		ID:       resp.ID,
		UUID:     resp.UUID,
		Slug:     resp.Slug,
		Name:     resp.Name,
		IsSystem: resp.IsSystem,
	}
	if resp.Description != nil {
		detail.Description = resp.Description
	}
	if len(resp.Permissions) > 0 {
		detail.Permissions = mapRoleDetailPermissions(resp.Permissions)
	}

	return detail
}

func mapRolePermissions(perms []integrationroles.ListRolePermissionsPermission) []permissionDetail {
	res := make([]permissionDetail, 0, len(perms))
	for _, p := range perms {
		res = append(res, permissionDetail{
			Domain:      p.Domain,
			Action:      p.Action,
			Description: p.Description,
		})
	}

	return res
}

func mapRoleDetailPermissions(perms []integrationroles.GetRolePermission) []permissionDetail {
	res := make([]permissionDetail, 0, len(perms))
	for _, p := range perms {
		res = append(res, permissionDetail{
			Domain:      p.Domain,
			Action:      p.Action,
			Description: p.Description,
		})
	}

	return res
}

func mapPermissionRecords(perms []integrationroles.ListPermissionsPermission) []permissionRecord {
	res := make([]permissionRecord, 0, len(perms))
	for _, p := range perms {
		res = append(res, permissionRecord{
			ID:          p.ID,
			UUID:        p.UUID,
			Domain:      p.Domain,
			Action:      p.Action,
			Description: p.Description,
		})
	}

	return res
}

func (h *Handler) getAssignedAdminIDs(ctx context.Context, roleID int64) (map[int64]struct{}, error) {
	if roleID <= 0 {
		return nil, integrationroles.ErrInvalidRoleID
	}

	resp, err := h.listAllRoles.Handle(ctx, integrationroles.ListAllRolesRequest{})
	if err != nil {
		return nil, err
	}

	result := make(map[int64]struct{}, len(resp.SubjectRoles))
	for _, subjectRole := range resp.SubjectRoles {
		if subjectRole.RoleID != roleID {
			continue
		}

		adminID, ok := h.adminIDFromSubjectID(ctx, subjectRole.SubjectID)
		if ok {
			result[adminID] = struct{}{}
		}
	}

	return result, nil
}

func (h *Handler) adminIDFromSubjectID(ctx context.Context, subjectID string) (int64, bool) {
	parsedSubjectID, err := integrationroles.ParseSubjectIDString(subjectID)
	if err != nil {
		return 0, false
	}

	admin, err := h.adminRepo.GetBySubjectID(ctx, parsedSubjectID)
	if err != nil || admin.ID <= 0 {
		if err != nil {
			h.logger.WarnContext(ctx, "load assigned admin by subject", logger.Error(err))
		}
		return 0, false
	}

	return admin.ID, true
}

func (h *Handler) listRoleAdmins(ctx context.Context, roleID int64) ([]roleAssignedAdmin, error) {
	if roleID <= 0 {
		return nil, integrationroles.ErrInvalidRoleID
	}

	adminIDs, err := h.getAssignedAdminIDs(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("get assigned admins: %w", err)
	}

	if len(adminIDs) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(adminIDs))
	for adminID := range adminIDs {
		ids = append(ids, adminID)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	assigned := make([]roleAssignedAdmin, 0, len(ids))
	for _, adminID := range ids {
		admin, err := h.adminRepo.GetByID(ctx, adminID)
		if err != nil {
			h.logger.WarnContext(ctx, "load assigned admin", logger.Error(err))
			continue
		}
		assigned = append(assigned, toRoleAssignedAdmin(admin))
	}

	return assigned, nil
}

func (h *Handler) loadAdminRoles(ctx context.Context, adminID int64) ([]integrationroles.Role, error) {
	if adminID <= 0 {
		return nil, errInvalidAdminID
	}

	resp, err := h.listAdminRoles.Handle(ctx, ListAdminRolesRequest{AdminID: adminID})
	if err != nil {
		return nil, err
	}

	return resp.Roles, nil
}

func adminRoleListHasSlug(roles []integrationroles.Role, slug string) bool {
	if slug == "" {
		return false
	}
	for _, role := range roles {
		if strings.EqualFold(role.Slug, slug) {
			return true
		}
	}
	return false
}

func toRoleAssignedAdmin(admin models.Admin) roleAssignedAdmin {
	return roleAssignedAdmin{
		ID:       admin.ID,
		Name:     admin.Name,
		LastName: admin.LastName,
		Email:    admin.Email,
		Username: admin.Username,
	}
}

func uniqueRoleIDs(roles []integrationroles.Role) []int64 {
	seen := make(map[int64]struct{}, len(roles))
	result := make([]int64, 0, len(roles))
	for _, role := range roles {
		if role.ID <= 0 {
			continue
		}
		if _, ok := seen[role.ID]; ok {
			continue
		}
		seen[role.ID] = struct{}{}
		result = append(result, role.ID)
	}
	return result
}

func containsRoleID(roleIDs []int64, target int64) bool {
	for _, id := range roleIDs {
		if id == target {
			return true
		}
	}
	return false
}

func (h *Handler) hasSuperRoleIDs(ctx context.Context, roleIDs []int64) (bool, error) {
	if len(roleIDs) == 0 {
		return false, nil
	}

	checked := make(map[int64]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		if roleID <= 0 {
			continue
		}

		if _, ok := checked[roleID]; ok {
			continue
		}
		checked[roleID] = struct{}{}

		detail, err := h.fetchRoleDetail(ctx, roleID, false)
		if err != nil {
			return false, err
		}

		if detail.Slug == integrationroles.SuperAdminRole {
			return true, nil
		}
	}

	return false, nil
}

func extractRoleNames(roles []integrationroles.Role) []string {
	result := make([]string, 0, len(roles))
	for _, r := range roles {
		if r.Name != "" {
			result = append(result, r.Name)
		}
	}
	return result
}

func mapRoleDomainError(err error, fallback string) (string, bool) {
	switch {
	case errors.Is(err, integrationroles.ErrRoleNotFound):
		return "Роль не найдена", true
	case errors.Is(err, integrationroles.ErrPermissionNotFound):
		return "Права не найдены", true
	case errors.Is(err, integrationroles.ErrInvalidRoleID):
		return "Некорректный идентификатор роли", true
	case errors.Is(err, integrationroles.ErrInvalidRoleSlug):
		return "Некорректный slug", true
	case errors.Is(err, integrationroles.ErrInvalidPermission):
		return "Некорректный ключ права", true
	case errors.Is(err, errInvalidAdminID):
		return "Некорректный идентификатор администратора", true
	case errors.Is(err, integrationroles.ErrSystemRoleProtected):
		return "Системные роли защищены от изменений", true
	case errors.Is(err, integrationroles.ErrInvalidRoleName):
		return "Некорректное имя роли", true
	default:
		return fallback, false
	}
}

func (h *Handler) withAdminIDError(c fiber.Ctx, message string) {
	h.adminApp.HTTPManager().WithErrors(c, map[string]string{
		"adminId": message,
	})
}

//nolint:unparam // it's valid
func (h *Handler) respondAdminDomainError(c fiber.Ctx, err error, fallback string, status int) error {
	if isInertiaRequest(c) {
		message, _ := mapRoleDomainError(err, fallback)
		h.withAdminIDError(c, message)
		h.flashDomainError(c, err, fallback)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if err != nil {
		return goinertia.NewError(status, fallback, err)
	}

	return goinertia.NewError(status, fallback)
}

func (h *Handler) respondAdminFormError(c fiber.Ctx, message string, status int, err error) error {
	if isInertiaRequest(c) {
		h.withAdminIDError(c, message)
		if err != nil {
			h.logger.ErrorContext(c, message, logger.Error(err))
		}
		h.adminApp.HTTPManager().WithFlashError(c, message)
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	if err != nil {
		return goinertia.NewError(status, message, err)
	}

	return goinertia.NewError(status, message)
}

func (h *Handler) respondAttachSuccess(c fiber.Ctx, admin models.Admin) error {
	if isInertiaRequest(c) {
		h.adminApp.HTTPManager().WithFlashSuccess(c, "Роль назначена администратору")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	return c.JSON(fiber.Map{"admin": toRoleAssignedAdmin(admin)})
}

func (h *Handler) respondDetachSuccess(c fiber.Ctx, adminID int64) error {
	if isInertiaRequest(c) {
		h.adminApp.HTTPManager().WithFlashSuccess(c, "Роль откреплена от администратора")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	return c.JSON(fiber.Map{"adminId": adminID})
}

func isInertiaRequest(c fiber.Ctx) bool {
	return c.Get(goinertia.HeaderInertia) != ""
}
