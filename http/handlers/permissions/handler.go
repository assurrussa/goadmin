package permissions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

//go:generate toolsmocks

type listPermissionsUseCase interface {
	Handle(ctx context.Context, req integrationroles.ListPermissionsRequest) (integrationroles.ListPermissionsResponse, error)
}

type permissionService interface {
	EnsurePermissions(ctx context.Context, inputs []integrationroles.CreatePermissionInput) error
	CreatePermissions(ctx context.Context, inputs []integrationroles.CreatePermissionInput) ([]integrationroles.Permission, error)
}

type Handler struct {
	adminApp        *adminapp.App
	routePath       string
	logger          logger.Logger
	dataGridHandler *datagrid.Handler[permissionGridItem]

	listPermissions listPermissionsUseCase
	service         permissionService
}

func NewHandler(
	adminApp *adminapp.App,
	listPermissions listPermissionsUseCase,
	service permissionService,
) *Handler {
	routePath := "/permissions"
	gridHandler := datagrid.NewHandler(datagrid.FilterConfig[permissionGridItem]{
		RoutePath:         routePath,
		Entity:            integrationroles.PermissionDomainPermissions.String(),
		Title:             "Справочник разрешений",
		Repository:        newPermissionsGridRepository(listPermissions),
		DefaultSort:       "domain",
		DefaultOrder:      "asc",
		PageSize:          50,
		Creatable:         false,
		Refreshable:       true,
		Exportable:        false,
		SearchPlaceholder: "Поиск по домену/действию",
		IDKey:             "id",
		Columns: []datagrid.Column{
			{Key: "domain", Label: "Домен", Type: "text", Sortable: true, Filterable: true, FilterPlaceholder: "roles"}, //nolint:goconst,lll // required
			{Key: "action", Label: "Действие", Type: "text", Sortable: true, Filterable: true, FilterPlaceholder: "read"},
			{Key: "description", Label: "Описание", Type: "text"},
		},
		Actions: []datagrid.Action[permissionGridItem]{
			{Key: "sync", Label: "Синхронизировать", Icon: "rotate", Variant: "primary"},
		},
		PageComponent: func(c fiber.Ctx, data datagrid.Response[permissionGridItem]) error {
			return adminApp.HTTPManager().Render(c, "permissions/IndexPage", map[string]any{
				"title": data.Title,
				"data":  data.ToAPIResponse(),
			})
		},
	}, adminApp.Logger())

	return &Handler{
		adminApp:        adminApp,
		routePath:       routePath,
		logger:          adminApp.Logger().WithNamed("permissions-handler"),
		dataGridHandler: gridHandler,
		listPermissions: listPermissions,
		service:         service,
	}
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	group := route.Group(h.routePath, h.adminApp.Guard(integrationroles.PermissionDomainPermissions, integrationroles.PermissionActionRead)) //nolint:lll // required
	h.dataGridHandler.RegisterRoutes(group, "")
	group.Post("refresh", h.Refresh)
	group.Post("sync", h.adminApp.Guard(integrationroles.PermissionDomainPermissions, integrationroles.PermissionActionSync), h.Sync)
}

func (h *Handler) Refresh(c fiber.Ctx) error {
	h.adminApp.HTTPManager().WithFlashInfo(c, "Данные обновлены")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) Sync(c fiber.Ctx) error {
	defs := integrationroles.NewPermissionCatalog(h.adminApp.PermissionDefinitions())
	inputs := make([]integrationroles.CreatePermissionInput, 0, len(defs.Data))
	for _, def := range defs.Data {
		inputs = append(inputs, integrationroles.CreatePermissionInput(def))
	}

	if err := h.service.EnsurePermissions(c, inputs); err != nil {
		h.logger.ErrorContext(c, "ensure permissions", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashError(c, "Не удалось синхронизировать справочник")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}
	if _, err := h.service.CreatePermissions(c, inputs); err != nil {
		h.logger.ErrorContext(c, "update permissions description", logger.Error(err))
		h.adminApp.HTTPManager().WithFlashWarning(c, "Права синхронизированы, но описания не обновлены")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Права успешно синхронизированы")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

type permissionGridItem struct {
	ID          int64                             `json:"id"`
	UUID        string                            `json:"uuid"`
	Domain      integrationroles.PermissionDomain `json:"domain"`
	Action      integrationroles.PermissionAction `json:"action"`
	Description *string                           `json:"description,omitempty"`
}

type permissionsGridRepository struct {
	useCase listPermissionsUseCase
}

func newPermissionsGridRepository(useCase listPermissionsUseCase) *permissionsGridRepository {
	return &permissionsGridRepository{useCase: useCase}
}

func (r *permissionsGridRepository) GetList(ctx context.Context, filtered datagrid.Filtered) ([]permissionGridItem, int, error) {
	resp, err := r.useCase.Handle(ctx, integrationroles.ListPermissionsRequest{})
	if err != nil {
		return nil, 0, err
	}

	items := make([]permissionGridItem, 0, len(resp.Permissions))
	for _, perm := range resp.Permissions {
		items = append(items, permissionGridItem{
			ID:          perm.ID,
			UUID:        perm.UUID,
			Domain:      perm.Domain,
			Action:      perm.Action,
			Description: perm.Description,
		})
	}

	filteredItems := filterPermissions(items, filtered)
	sortPermissions(filteredItems, filtered.GetSortBy(), filtered.GetSortOrder())
	total := len(filteredItems)
	paged := paginatePermissions(filteredItems, filtered.GetOffset(), filtered.GetLimit())

	return paged, total, nil
}

func filterPermissions(items []permissionGridItem, filtered datagrid.Filtered) []permissionGridItem {
	if len(items) == 0 {
		return items
	}

	fields := filtered.GetFields()
	search := strings.TrimSpace(strings.ToLower(filtered.GetSearch()))
	domainFilter := strings.TrimSpace(strings.ToLower(fieldValue(fields, "domain")))
	actionFilter := strings.TrimSpace(strings.ToLower(fieldValue(fields, "action")))

	if search == "" && domainFilter == "" && actionFilter == "" {
		return items
	}

	result := make([]permissionGridItem, 0, len(items))
	for _, item := range items {
		domain := strings.ToLower(item.Domain.String())
		action := strings.ToLower(item.Action.String())
		if search != "" {
			descr := ""
			if item.Description != nil {
				descr = *item.Description
			}
			if !strings.Contains(domain, search) &&
				!strings.Contains(action, search) &&
				!strings.Contains(strings.ToLower(descr), search) {
				continue
			}
		}
		if domainFilter != "" && !strings.Contains(domain, domainFilter) {
			continue
		}
		if actionFilter != "" && !strings.Contains(action, actionFilter) {
			continue
		}

		result = append(result, item)
	}

	return result
}

func sortPermissions(items []permissionGridItem, sortBy, sortOrder string) {
	if len(items) == 0 {
		return
	}

	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	desc := strings.EqualFold(sortOrder, "desc")

	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return comparePermissions(items[j], items[i], sortBy)
		}
		return comparePermissions(items[i], items[j], sortBy)
	})
}

func comparePermissions(a, b permissionGridItem, sortBy string) bool {
	switch sortBy {
	case "action":
		if strings.EqualFold(a.Action.String(), b.Action.String()) {
			return strings.ToLower(a.Domain.String()) < strings.ToLower(b.Domain.String())
		}
		return strings.ToLower(a.Action.String()) < strings.ToLower(b.Action.String())
	case "description":
		descrA := ""
		descrB := ""
		if a.Description != nil {
			descrA = *a.Description
		}
		if b.Description != nil {
			descrB = *b.Description
		}
		if strings.EqualFold(descrA, descrB) {
			return strings.ToLower(a.Domain.String()) < strings.ToLower(b.Domain.String())
		}
		return strings.ToLower(descrA) < strings.ToLower(descrB)
	default:
		if strings.EqualFold(a.Domain.String(), b.Domain.String()) {
			return strings.ToLower(a.Action.String()) < strings.ToLower(b.Action.String())
		}
		return strings.ToLower(a.Domain.String()) < strings.ToLower(b.Domain.String())
	}
}

func paginatePermissions(items []permissionGridItem, offset, limit int) []permissionGridItem {
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

	sliced := make([]permissionGridItem, limit)
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
