package users

import (
	"context"
	"errors"
	"strconv"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/internal/admintx"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/tablecommands"
	"github.com/assurrussa/goadmin/shared"
)

const (
	userDeleteAction  = "delete"
	userRestoreAction = "restore"
)

func (h *Handler) commandUser(ctx context.Context, id int64, excludeAdmins bool) (authcore.Profile, error) {
	user, err := h.userRepo.GetByID(ctx, id)
	if err != nil {
		return user, err
	}
	if user.ID == 0 || user.AuthSubjectID().IsZero() {
		return user, fiber.ErrNotFound
	}
	if err = h.lockCommandSubject(ctx, user.AuthSubjectID(), excludeAdmins); err != nil {
		return user, err
	}
	if h.adminApp.GridDB() == nil {
		return user, nil
	}
	// Re-read after waiting for a concurrent command's subject lock.
	return h.userRepo.GetByID(ctx, id)
}

func (h *Handler) lockCommandSubject(ctx context.Context, subject goauth.SubjectID, excludeAdmins bool) error {
	db := h.adminApp.GridDB()
	if db == nil {
		if excludeAdmins {
			return errors.New("user bulk storage is unavailable")
		}
		return nil
	}
	var status string
	if err := admintx.Wrap(db).ScanOne(ctx, "user.command.lock", &status,
		`SELECT status FROM auth_subjects WHERE id=$1 FOR UPDATE`, subject); err != nil {
		return err
	}
	if !excludeAdmins {
		return nil
	}
	var membership bool
	err := admintx.Wrap(db).ScanOne(ctx, "user.command.membership", &membership,
		`SELECT EXISTS(SELECT 1 FROM administrations WHERE subject_id=$1 AND deleted_at IS NULL)`, subject)
	if err != nil {
		return err
	}
	if membership {
		return tablecommands.ErrSkipped
	}
	return nil
}

func (h *Handler) changeUserStatus(c fiber.Ctx, id int64, restore, excludeAdmins bool) error {
	status, action := goauth.SubjectStatusDisabled, "user.soft_deleted"
	if restore {
		status, action = goauth.SubjectStatusActive, "user.restored"
	}
	audit := h.adminApp.GridAudit()
	record := adminactionauditrepo.Record{Action: action, TargetType: "user", TargetID: strconv.FormatInt(id, 10)}
	if audit != nil {
		actor := shared.MustGetAdminAuth(c)
		record.ActorAdminID, record.ActorSubjectID = actor.ID, actor.AuthSubjectID().String()
		record.RequestID = requestid.FromContext(c)
	}
	return h.adminApp.InTransaction(c, func(ctx context.Context) error {
		user, err := h.commandUser(ctx, id, excludeAdmins)
		if err != nil {
			return err
		}
		if user.ID == 0 {
			return fiber.ErrNotFound
		}
		if excludeAdmins && user.DeletedAt.Valid != restore {
			return tablecommands.ErrSkipped
		}
		if _, err = h.subjects.SetSubjectStatus(ctx, user.AuthSubjectID(), status); err != nil {
			return err
		}
		if restore {
			err = h.userRepo.RestoreByID(ctx, id)
		} else {
			err = h.userRepo.SoftDeleteByID(ctx, id)
		}
		if err != nil {
			return err
		}
		if audit != nil {
			return audit.RecordAdminAction(ctx, record)
		}
		return nil
	})
}

func (h *Handler) Bulk(c fiber.Ctx) error {
	request, err := tablecommands.Parse(c.Body(), []string{userDeleteAction, userRestoreAction}, tablecommands.DecimalID)
	if err != nil {
		return err
	}
	return tablecommands.Run(c, request, func(id string) (string, error) {
		value, _ := strconv.ParseInt(id, 10, 64)
		return "", h.changeUserStatus(c, value, request.Action == userRestoreAction, true)
	})
}
