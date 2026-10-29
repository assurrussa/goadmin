package bootstrap //nolint:testpackage // checks optional module assembly using narrow repository doubles.

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	auth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/dashboard"
)

const (
	testDashboardQueuesModule        = "queues"
	testDashboardNotificationsModule = "notifications"
	testDashboardQueues              = "goadmin.queues"
	testDashboardNotifications       = "goadmin.notifications"
	testDashboardSubject             = "dashboard-subject"
	testDashboardReady               = "ready"
	testDashboardUnavailable         = "unavailable"
)

type dashboardJobs struct {
	JobsRepo
	count int64
	err   error
}

func (r dashboardJobs) CountExact(context.Context) (int64, error) { return r.count, r.err }

type dashboardFailedJobs struct {
	JobsFailedRepo
	count int64
	err   error
}

func (r dashboardFailedJobs) CountExact(context.Context) (int64, error) { return r.count, r.err }

type dashboardNotifications struct {
	NotificationsRepo
	err error
}

func (r dashboardNotifications) CountUnread(_ context.Context, id int64) (int, error) {
	return int(id), r.err
}

func TestBuiltInDashboardUsesExplicitModulesAndScopedRepositories(t *testing.T) {
	deps := Dependencies{
		Repos:  AppRepositories{JobsRepo: dashboardJobs{count: 12}, JobsFailedRepo: dashboardFailedJobs{count: 3}},
		System: SystemDependencies{AdminNotificationRepo: dashboardNotifications{}},
	}
	for _, tc := range []struct {
		name          string
		modules       map[string]bool
		queuesAllowed bool
		keys          []string
	}{
		{"core", map[string]bool{}, true, nil},
		{"jobs alone", map[string]bool{"jobs": true}, true, nil},
		{testDashboardQueuesModule, map[string]bool{testDashboardQueuesModule: true}, true, []string{testDashboardQueues}},
		{"queues forbidden", map[string]bool{testDashboardQueuesModule: true}, false, nil},
		{
			testDashboardNotificationsModule,
			map[string]bool{testDashboardNotificationsModule: true},
			false,
			[]string{testDashboardNotifications},
		},
		{
			"both",
			map[string]bool{testDashboardQueuesModule: true, testDashboardNotificationsModule: true},
			true,
			[]string{testDashboardQueues, testDashboardNotifications},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry, err := dashboard.New(builtInDashboardCards(Config{Capabilities: tc.modules}, deps), nil)
			require.NoError(t, err)
			for _, id := range []int64{4, 8} {
				request := registry.Authorize(dashboard.Actor{AdminID: id, SubjectID: testDashboardSubject},
					func(auth.PermissionKey) bool { return tc.queuesAllowed })
				descriptors := request.Descriptors()
				require.Len(t, descriptors.Cards, len(tc.keys))
				for i, key := range tc.keys {
					require.Equal(t, key, descriptors.Cards[i].Key)
					require.Equal(t, "loading", descriptors.Cards[i].State)
				}
				loaded := request.Load(t.Context(), nil)
				for _, card := range loaded.Cards {
					require.Equal(t, testDashboardReady, card.State)
					if card.Key == testDashboardQueues {
						require.Equal(t, "12", card.Metrics[0].Value)
						require.Equal(t, "Задач в очереди", card.Metrics[0].Label.Text)
						require.Equal(t, "3", card.Metrics[1].Value)
						require.Equal(t, "Неудачных задач", card.Metrics[1].Label.Text)
						require.Equal(t, "warning", card.Status.Tone)
						require.Equal(t, "/queues/jobs", card.Links[0].Href)
						require.Equal(t, "/queues/jobs-failed", card.Links[1].Href)
					} else {
						require.Equal(t, strconv.FormatInt(id, 10), card.Metrics[0].Value)
						require.Equal(t, "/notifications?status=unread", card.Links[0].Href)
					}
				}
			}
		})
	}
}

func TestBuiltInDashboardRepositoryFailuresAndZeroCounts(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		jobsErr, failedErr, notificationsErr error
		queueState, notificationState        string
	}{
		{name: "zero", queueState: testDashboardReady, notificationState: testDashboardReady},
		{
			name: "jobs error", jobsErr: errors.New("jobs unavailable"),
			queueState: testDashboardUnavailable, notificationState: testDashboardReady,
		},
		{
			name: "failed error", failedErr: errors.New("failed jobs unavailable"),
			queueState: testDashboardUnavailable, notificationState: testDashboardReady,
		},
		{
			name: "notification error", notificationsErr: errors.New("inbox unavailable"),
			queueState: testDashboardReady, notificationState: testDashboardUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := Dependencies{
				Repos:  AppRepositories{JobsRepo: dashboardJobs{err: tc.jobsErr}, JobsFailedRepo: dashboardFailedJobs{err: tc.failedErr}},
				System: SystemDependencies{AdminNotificationRepo: dashboardNotifications{err: tc.notificationsErr}},
			}
			cfg := Config{Capabilities: map[string]bool{testDashboardQueuesModule: true, testDashboardNotificationsModule: true}}
			registry, err := dashboard.New(builtInDashboardCards(cfg, deps), nil)
			require.NoError(t, err)
			request := registry.Authorize(dashboard.Actor{AdminID: 4, SubjectID: testDashboardSubject},
				func(auth.PermissionKey) bool { return true })
			loaded := request.Load(t.Context(), nil)
			require.Equal(t, tc.queueState, loaded.Cards[0].State)
			require.Nil(t, loaded.Cards[0].Status, "zero jobs does not assert healthy workers")
			require.Equal(t, tc.notificationState, loaded.Cards[1].State)
			if tc.queueState == testDashboardUnavailable {
				require.Empty(t, loaded.Cards[0].Metrics)
			}
			if tc.notificationState == testDashboardUnavailable {
				require.Empty(t, loaded.Cards[1].Metrics)
			}
		})
	}
}

type dashboardExtension struct{ cards []dashboard.Card }

func (e dashboardExtension) DashboardCards() []dashboard.Card { return e.cards }

func TestDashboardExtensionsValidateAssembledRegistrations(t *testing.T) {
	card := dashboard.Card{
		Key: "host.reports", Title: dashboard.Text{Text: "Reports"},
		PermissionKey: auth.NewPermissionKey("reports", "read"),
		Load:          func(context.Context, dashboard.Actor) (dashboard.Content, error) { return dashboard.Content{}, nil },
	}
	builders := []ExtensionBuilder{
		func(*adminapp.App) (Extension, error) { return dashboardExtension{cards: []dashboard.Card{card}}, nil },
		func(*adminapp.App) (Extension, error) { return dashboardExtension{cards: []dashboard.Card{card}}, nil },
	}
	extensions, err := buildServerExtensions(nil, builders)
	require.NoError(t, err)
	_, err = dashboard.New(nil, extensions.dashboardCards)
	require.ErrorContains(t, err, "duplicate")
	card.Key = testDashboardNotifications
	_, err = dashboard.New(nil, []dashboard.Card{card})
	require.ErrorContains(t, err, "invalid dashboard card key")
}
