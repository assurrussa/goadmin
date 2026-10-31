// Package tablecommands implements bounded, explicit-ID table commands.
package tablecommands

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
)

const MaxSelected = 100

var ErrSkipped = errors.New("record is not eligible for this action")

type Request struct {
	Action string   `json:"action"`
	IDs    []string `json:"ids"`
}

type Result struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Code      string `json:"code,omitempty"`
	RelatedID string `json:"relatedId,omitempty"`
}

func DecimalID(value string) (string, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return "", fiber.ErrBadRequest
	}
	return value, nil
}

// Parse validates every identifier before any mutation and preserves first-seen order.
func Parse(body []byte, actions []string, normalize func(string) (string, error)) (Request, error) {
	var request Request
	if len(body) > 64*1024 || json.Unmarshal(body, &request) != nil || len(request.IDs) == 0 || len(request.IDs) > MaxSelected {
		return request, fiber.ErrBadRequest
	}
	allowed := false
	for _, action := range actions {
		if request.Action == action {
			allowed = true
		}
	}
	if !allowed {
		return request, fiber.ErrBadRequest
	}
	seen := make(map[string]bool)
	ids := make([]string, 0, len(request.IDs))
	for _, raw := range request.IDs {
		id, err := normalize(strings.TrimSpace(raw))
		if err != nil {
			return request, fiber.ErrBadRequest
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	request.IDs = ids
	return request, nil
}

// Run never retries a command. Unexpected operational errors are conservatively
// unknown: a lost commit acknowledgement cannot prove rollback.
func Run(c fiber.Ctx, request Request, mutate func(string) (string, error)) error {
	results := make([]Result, 0, len(request.IDs))
	stopped := false
	for _, id := range request.IDs {
		result := Result{ID: id, Status: "not_processed"}
		if stopped || c.Err() != nil {
			results = append(results, result)
			continue
		}
		related, err := mutate(id)
		result.RelatedID = related
		switch {
		case err == nil:
			result.Status = "succeeded"
		case errors.Is(err, ErrSkipped), errors.Is(err, fiber.ErrNotFound), errors.Is(err, fiber.ErrForbidden):
			result.Status, result.Code = "skipped", "not_eligible"
		case errors.Is(err, fiber.ErrBadRequest):
			result.Status, result.Code = "failed", "invalid_record"
		case errors.Is(err, goauth.ErrOperationOutcomeUnknown):
			result.Status, result.Code, stopped = "unknown", "outcome_unknown", true
		default:
			result.Status, result.Code, stopped = "unknown", "operation_unavailable", true
		}
		results = append(results, result)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(struct {
		Results []Result `json:"results"`
	}{results})
}
