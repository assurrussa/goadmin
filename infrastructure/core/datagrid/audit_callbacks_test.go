//nolint:goconst // Independent contract fixtures intentionally repeat callback event literals.
package datagrid //nolint:testpackage // Internal methods isolate callback ordering and failure short-circuits.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

type auditCallbackRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type auditCallbackRepo func(context.Context, Filtered) ([]auditCallbackRow, int, error)

func (r auditCallbackRepo) GetList(ctx context.Context, f Filtered) ([]auditCallbackRow, int, error) {
	return r(ctx, f)
}

func TestAuditCallbackOrderArgumentsAndMerge(t *testing.T) {
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)
	ctx.Locals("audit-request", "same-request")
	rows := []auditCallbackRow{{ID: 2, Name: "two"}, {ID: 1, Name: "one"}}
	var events []string
	checkCtx := func(got context.Context) {
		require.Same(t, ctx, got)
		require.Equal(t, "same-request", got.Value("audit-request"))
	}
	cfg := FilterConfig[auditCallbackRow]{
		Repository: auditCallbackRepo(func(got context.Context, filters Filtered) ([]auditCallbackRow, int, error) {
			checkCtx(got)
			require.Equal(t, 10, filters.GetLimit())
			events = append(events, "repository")
			return rows, 2, nil
		}),
		Actions: []Action[auditCallbackRow]{
			{Key: "always"},
			{Key: "first", CanView: func(got context.Context, row auditCallbackRow) bool {
				checkCtx(got)
				events = append(events, fmt.Sprintf("action-first:%d", row.ID))
				return row.ID == 2
			}},
			{Key: "second", CanView: func(got context.Context, row auditCallbackRow) bool {
				checkCtx(got)
				events = append(events, fmt.Sprintf("action-second:%d", row.ID))
				return true
			}},
		},
		RowDecorators: []RowDecorator[auditCallbackRow]{
			NewRowDecorator(func(got context.Context, data []auditCallbackRow) error {
				checkCtx(got)
				require.Equal(t, rows, data)
				events = append(events, "prepare-first")
				return nil
			}, func(got context.Context, row auditCallbackRow) (map[string]any, error) {
				checkCtx(got)
				events = append(events, fmt.Sprintf("resolve-first:%d", row.ID))
				return map[string]any{"shared": "first", "id": row.ID}, nil
			}),
			NewRowDecorator(func(got context.Context, data []auditCallbackRow) error {
				checkCtx(got)
				require.Equal(t, rows, data)
				events = append(events, "prepare-second")
				return nil
			}, func(got context.Context, row auditCallbackRow) (map[string]any, error) {
				checkCtx(got)
				events = append(events, fmt.Sprintf("resolve-second:%d", row.ID))
				return map[string]any{"shared": "second", "name": "computed"}, nil
			}),
		},
	}
	h := NewHandler(cfg, logger.Discard())
	resp, err := h.loadDataInternal(ctx, Filters{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, []string{
		"repository", "prepare-first", "prepare-second",
		"action-first:2", "action-second:2", "resolve-first:2", "resolve-second:2",
		"action-first:1", "action-second:1", "resolve-first:1", "resolve-second:1",
	}, events)
	require.Equal(t, rows[0], resp.Items[0].Item)
	require.Equal(t, map[string]any{"shared": "second", "id": 2, "name": "computed"}, resp.Items[0].Values)
	require.Equal(t, []string{"always", "first", "second"}, []string{
		resp.Items[0].Actions[0].Key, resp.Items[0].Actions[1].Key, resp.Items[0].Actions[2].Key,
	})
	require.Equal(t, []string{"always", "second"}, []string{resp.Items[1].Actions[0].Key, resp.Items[1].Actions[1].Key})
}

func TestAuditCallbackErrorsStopWorkWithoutPartialResponse(t *testing.T) {
	for _, stage := range []string{"repository", "prepare", "resolve"} {
		t.Run(stage, func(t *testing.T) {
			app := fiber.New()
			ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
			defer app.ReleaseCtx(ctx)
			failure := errors.New("audit failure")
			var events []string
			cfg := FilterConfig[auditCallbackRow]{
				Repository: auditCallbackRepo(func(context.Context, Filtered) ([]auditCallbackRow, int, error) {
					events = append(events, "repository")
					if stage == "repository" {
						return nil, 0, failure
					}
					return []auditCallbackRow{{ID: 1}, {ID: 2}}, 2, nil
				}),
				Actions: []Action[auditCallbackRow]{{Key: "check", CanView: func(context.Context, auditCallbackRow) bool {
					events = append(events, "action")
					return true
				}}},
				RowDecorators: []RowDecorator[auditCallbackRow]{
					NewRowDecorator(func(context.Context, []auditCallbackRow) error {
						events = append(events, "prepare")
						if stage == "prepare" {
							return failure
						}
						return nil
					}, func(context.Context, auditCallbackRow) (map[string]any, error) {
						events = append(events, "resolve")
						return nil, failure
					}),
					NewRowDecoratorResolve(func(context.Context, auditCallbackRow) (map[string]any, error) {
						events = append(events, "unexpected-second-resolve")
						return nil, nil //nolint:nilnil // A decorator may deliberately supply no computed values.
					}),
				},
			}
			resp, err := NewHandler(cfg, logger.Discard()).loadDataInternal(ctx, Filters{Page: 1, Limit: 10})
			require.ErrorIs(t, err, failure)
			require.Equal(t, Response[auditCallbackRow]{}, resp)
			switch stage {
			case "repository":
				require.Equal(t, []string{"repository"}, events)
			case "prepare":
				require.Equal(t, []string{"repository", "prepare"}, events)
				require.Contains(t, err.Error(), "row decorator prepare")
			case "resolve":
				require.Equal(t, []string{"repository", "prepare", "action", "resolve"}, events)
				require.Contains(t, err.Error(), "row decorator resolve")
			}
		})
	}
}

func TestAuditEmptyRowsSkipAllCallbacksAndNilActionsSerializeNull(t *testing.T) {
	calls := 0
	cfg := FilterConfig[auditCallbackRow]{
		Repository: auditCallbackRepo(func(context.Context, Filtered) ([]auditCallbackRow, int, error) { return nil, 0, nil }),
		Actions:    []Action[auditCallbackRow]{{CanView: func(context.Context, auditCallbackRow) bool { calls++; return true }}},
		RowDecorators: []RowDecorator[auditCallbackRow]{NewRowDecorator(
			func(context.Context, []auditCallbackRow) error { calls++; return nil },
			func(context.Context, auditCallbackRow) (map[string]any, error) {
				calls++
				return nil, nil //nolint:nilnil // A decorator may deliberately supply no computed values.
			},
		)},
	}
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)
	resp, err := NewHandler(cfg, logger.Discard()).loadDataInternal(ctx, Filters{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 0, calls)
	require.NotNil(t, resp.Items)
	require.Empty(t, resp.Items)

	cfg.Actions = nil
	h := NewHandler(cfg, logger.Discard())
	items, err := h.buildItems(ctx, []auditCallbackRow{{ID: 1}})
	require.NoError(t, err)
	require.Nil(t, items[0].Actions)
	require.Nil(t, items[0].Values)
	bs, err := (Response[auditCallbackRow]{Items: items}).ToJSON()
	require.NoError(t, err)
	require.Contains(t, string(bs), `"actions":null`)
	require.NotContains(t, string(bs), `"values"`)
}

func TestAuditCustomErrorComponentOwnsHTTPStatus(t *testing.T) {
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)
	failure := errors.New("repository failure")
	calls := 0
	h := NewHandler(FilterConfig[auditCallbackRow]{
		Title: "Audit", Entity: "audit",
		ErrorComponent: func(got fiber.Ctx, data ErrorData, err error) error {
			calls++
			require.Same(t, ctx, got)
			require.ErrorIs(t, err, failure)
			require.Equal(t, 500, data.Status)
			return got.SendString("custom error")
		},
	}, logger.Discard())
	require.NoError(t, h.renderErrorPage(ctx, failure))
	require.Equal(t, 1, calls)
	require.Equal(t, 200, ctx.Response().StatusCode())
}

// Bounded using -benchtime=50x. Includes one in-memory repository load, two
// batched prepares, three per-row predicates, two resolves and JSON encoding.
// Excludes database latency, network transport and browser rendering.
func BenchmarkAuditGridResponse(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
			rows := make([]auditCallbackRow, count)
			for i := range rows {
				rows[i] = auditCallbackRow{ID: i + 1, Name: "audit row"}
			}
			var repoCalls, prepareCalls, actionCalls, resolveCalls int
			predicate := func(context.Context, auditCallbackRow) bool { actionCalls++; return true }
			prepare := func(context.Context, []auditCallbackRow) error { prepareCalls++; return nil }
			resolve := func(_ context.Context, row auditCallbackRow) (map[string]any, error) {
				resolveCalls++
				return map[string]any{"computed": row.ID}, nil
			}
			h := NewHandler(FilterConfig[auditCallbackRow]{
				Repository: auditCallbackRepo(func(context.Context, Filtered) ([]auditCallbackRow, int, error) {
					repoCalls++
					return rows, count, nil
				}),
				Actions: []Action[auditCallbackRow]{
					{Key: "a", CanView: predicate}, {Key: "b", CanView: predicate}, {Key: "c", CanView: predicate},
				},
				RowDecorators: []RowDecorator[auditCallbackRow]{NewRowDecorator(prepare, resolve), NewRowDecorator(prepare, resolve)},
			}, logger.Discard())
			app := fiber.New()
			ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
			defer app.ReleaseCtx(ctx)
			filters := Filters{Page: 1, Limit: count}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp, err := h.loadDataInternal(ctx, filters)
				if err != nil {
					b.Fatal(err)
				}
				if _, err = resp.ToJSON(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if repoCalls != b.N || prepareCalls != 2*b.N || actionCalls != 3*count*b.N || resolveCalls != 2*count*b.N {
				b.Fatalf("callback counts: repo=%d prepare=%d actions=%d resolve=%d", repoCalls, prepareCalls, actionCalls, resolveCalls)
			}
			b.ReportMetric(float64(actionCalls)/float64(b.N), "actions/op")
			b.ReportMetric(float64(prepareCalls)/float64(b.N), "prepares/op")
			b.ReportMetric(float64(resolveCalls)/float64(b.N), "resolves/op")
		})
	}
}
