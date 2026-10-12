package auth

import (
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/authdelay"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type localMFAEnrollmentActions interface {
	BeginLocalAdminEnrollment(c fiber.Ctx, challengeID string) (adminservice.MFALoginEnrollmentView, error)
	CompleteLocalAdminEnrollment(
		c fiber.Ctx, challengeID, enrollmentID, code string,
	) (models.Admin, goauth.Account, error)
}

type localMFAEnrollmentForm struct {
	ChallengeID  string `json:"challengeID"`  //nolint:tagliatelle // Existing MFA browser protocol field.
	EnrollmentID string `json:"enrollmentID"` //nolint:tagliatelle // Existing MFA browser protocol field.
	Code         string `json:"code"`
}

func localMFAEnrollmentError(c fiber.Ctx) error {
	return c.Status(fiber.StatusBadRequest).JSON(map[string]string{
		"message": "Не удалось подтвердить настройку. Проверьте код или начните вход заново.",
	})
}

func (h *Handler) PostLocalMFAEnrollmentBegin(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	var form localMFAEnrollmentForm
	if err := c.Bind().Body(&form); err != nil {
		return err
	}
	if form.ChallengeID == "" || len(form.ChallengeID) > 256 {
		return localMFAEnrollmentError(c)
	}
	actions, ok := h.sessionStoreActions.(localMFAEnrollmentActions)
	if !ok {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	enrollment, err := actions.BeginLocalAdminEnrollment(c, form.ChallengeID)
	if err != nil {
		return localMFAEnrollmentError(c)
	}
	return c.JSON(map[string]any{"enrollment": enrollment})
}

func (h *Handler) PostLocalMFAEnrollmentConfirm(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	var form localMFAEnrollmentForm
	if err := c.Bind().Body(&form); err != nil {
		return err
	}
	if form.ChallengeID == "" || len(form.ChallengeID) > 256 ||
		form.EnrollmentID == "" || len(form.EnrollmentID) > 256 || form.Code == "" || len(form.Code) > 128 {
		return localMFAEnrollmentError(c)
	}
	authdelay.RunSleeper(c, h.sleep)
	actions, ok := h.sessionStoreActions.(localMFAEnrollmentActions)
	if !ok {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	admin, account, err := actions.CompleteLocalAdminEnrollment(
		c, form.ChallengeID, form.EnrollmentID, form.Code,
	)
	if err != nil {
		return localMFAEnrollmentError(c)
	}
	h.recordSuccessfulLogin(c, account.Subject.ID, admin)
	h.touchAdminLoginMetadata(c, admin.ID, time.Now().UTC())
	return c.JSON(map[string]string{"redirect": h.buildPostLoginRedirect(c, admin.ID)})
}
