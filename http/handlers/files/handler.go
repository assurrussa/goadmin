package files

import (
	"context"
	"net/http"
	"strconv"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	adminshared "github.com/assurrussa/goadmin/shared"
)

type handler interface {
	RegisterGroupRoutesWithGuards(prefix string, router fiber.Router, guards uploadhost.UploadRouteGuards)
}

// FileRepository is the subset of the uploads repository used by admin file routes.
type FileRepository interface {
	GetByID(ctx context.Context, id int64) (uploadhost.File, error)
	List(ctx context.Context, filters uploadhost.ListFilters) ([]uploadhost.File, int, error)
}

type managerFileLister interface {
	ListByManager(ctx context.Context, managerID int64, filters uploadhost.ListFilters) ([]uploadhost.File, int, error)
}

type managerScopedFileRepository struct {
	repo FileRepository
}

// NewManagerScopedFileRepository applies the conservative admin upload ownership
// policy to file reads. Generic CMS entity permissions are host-specific and are
// not available here, so admin routes expose only files whose manager is the
// authenticated admin. This can hide files shared across admins or attached to
// entities that the host considers shared; such sharing needs an explicit policy.
func NewManagerScopedFileRepository(repo FileRepository) FileRepository {
	return managerScopedFileRepository{repo: repo}
}

func (r managerScopedFileRepository) GetByID(ctx context.Context, id int64) (uploadhost.File, error) {
	file, err := r.repo.GetByID(ctx, id)
	if err != nil || !ownedByCurrentAdmin(ctx, file) {
		return uploadhost.File{}, err
	}

	return file, nil
}

func (r managerScopedFileRepository) List(
	ctx context.Context,
	filters uploadhost.ListFilters,
) ([]uploadhost.File, int, error) {
	if lister, ok := r.repo.(managerFileLister); ok {
		admin := adminshared.GetAdminAuth(ctx)
		if admin == nil || admin.ID <= 0 {
			return nil, 0, nil
		}
		files, total, err := lister.ListByManager(ctx, admin.ID, filters)
		if err != nil {
			return nil, 0, err
		}
		for _, file := range files {
			if !ownedByCurrentAdmin(ctx, file) {
				return nil, 0, fiber.NewError(http.StatusInternalServerError, "invalid file ownership result")
			}
		}
		return files, total, nil
	}

	const pageSize = 100
	pageFilters := filters
	pageFilters.Offset = 0
	pageFilters.Limit = pageSize
	owned := make([]uploadhost.File, 0, filters.Limit)
	ownedCount := 0
	for {
		files, total, err := r.repo.List(ctx, pageFilters)
		if err != nil {
			return nil, 0, err
		}
		for _, file := range files {
			if !ownedByCurrentAdmin(ctx, file) {
				continue
			}
			if ownedCount >= filters.Offset && len(owned) < filters.Limit {
				owned = append(owned, file)
			}
			ownedCount++
		}
		pageFilters.Offset += len(files)
		if len(files) == 0 || pageFilters.Offset >= total {
			break
		}
	}

	// gouploads does not filter by manager. Scan upstream pages before applying
	// the requested offset so foreign rows cannot hide later owned files.
	return owned, ownedCount, nil
}

func ownedByCurrentAdmin(ctx context.Context, file uploadhost.File) bool {
	admin := adminshared.GetAdminAuth(ctx)
	return admin != nil && admin.ID > 0 && file.ID > 0 && !file.DeletedAt.Valid &&
		file.ManagerID != nil && *file.ManagerID == admin.ID
}

// Handler объединяет все API обработчики.
type Handler struct {
	adminApp *adminapp.App
	handler  handler
	repo     FileRepository
}

// NewHandler создает новый главный API обработчик.
func NewHandler(adminApp *adminapp.App, handler handler, repo FileRepository) *Handler {
	return &Handler{
		adminApp: adminApp,
		handler:  handler,
		repo:     repo,
	}
}

// RegisterGroupRoutes регистрирует все API маршруты.
func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	readGuard := h.adminApp.Guard(integrationroles.PermissionDomainUploads, integrationroles.PermissionActionRead)
	deleteGuard := h.adminApp.Guard(integrationroles.PermissionDomainUploads, integrationroles.PermissionActionDelete)
	h.handler.RegisterGroupRoutesWithGuards("/files", route, uploadhost.UploadRouteGuards{
		Read:   []fiber.Handler{readGuard, h.requireOwnedFile},
		Create: []fiber.Handler{h.adminApp.Guard(integrationroles.PermissionDomainUploads, integrationroles.PermissionActionCreate)},
		Delete: []fiber.Handler{deleteGuard},
	})
}

func (h *Handler) requireOwnedFile(c fiber.Ctx) error {
	fileID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || fileID <= 0 {
		return c.Next()
	}

	file, err := h.repo.GetByID(c, fileID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load file"})
	}
	if file.ID == 0 {
		return c.SendStatus(http.StatusNotFound)
	}

	return c.Next()
}
