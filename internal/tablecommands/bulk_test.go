package tablecommands_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/tablecommands"
)

const deleteAction = "delete"

func TestParseRejectsEntireInvalidBatchBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"action":"delete","ids":[]}`,
		`{"action":"restore","ids":["1"]}`,
		`{"action":"delete","ids":["1","2x"]}`,
		`{"action":"delete","ids":["9007199254740993",2]}`,
	} {
		_, err := tablecommands.Parse([]byte(body),
			[]string{deleteAction}, tablecommands.DecimalID)
		require.Error(t, err)
	}
	request, err := tablecommands.Parse(
		[]byte(`{"action":"delete","ids":["9007199254740993","1","9007199254740993"]}`),
		[]string{deleteAction}, tablecommands.DecimalID)
	require.NoError(t, err)
	require.Equal(t, []string{"9007199254740993", "1"}, request.IDs)
}

func TestBulkRetainsPartialResultsAndStopsAfterUnknownCommit(t *testing.T) {
	t.Parallel()
	app := fiber.New()
	var calls []string
	app.Post("/bulk", func(c fiber.Ctx) error {
		request := tablecommands.Request{Action: deleteAction, IDs: []string{"1", "2", "3", "4"}}
		return tablecommands.Run(c, request, func(id string) (string, error) {
			calls = append(calls, id)
			switch id {
			case "1":
				return "", nil
			case "2":
				return "", tablecommands.ErrSkipped
			default:
				return "", goauth.ErrOperationOutcomeUnknown
			}
		})
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "POST", "/bulk", strings.NewReader("{}")))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	var body struct {
		Results []tablecommands.Result `json:"results"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, []string{"1", "2", "3"}, calls)
	require.Equal(t, "succeeded", body.Results[0].Status)
	require.Equal(t, "skipped", body.Results[1].Status)
	require.Equal(t, "unknown", body.Results[2].Status)
	require.Equal(t, "not_processed", body.Results[3].Status)
}
