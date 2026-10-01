package host_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

type notificationRoundTripper func(*http.Request) (*http.Response, error)

func (f notificationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewNotificationManagerValidatesTransportWithoutRequests(t *testing.T) {
	const (
		notificationProjectKey = "project-key"
		notificationHTTPSURL   = "https://notify.example.test"
		notificationHTTPURL    = "http://notify.example.test"
	)

	t.Parallel()
	for _, tc := range []struct {
		name, url, key, wantError string
		allowInsecure             bool
	}{
		{name: "https", url: notificationHTTPSURL, key: notificationProjectKey},
		{name: "remote HTTP denied", url: notificationHTTPURL, key: notificationProjectKey, wantError: "insecure http"},
		{name: "explicit insecure remote HTTP", url: notificationHTTPURL, key: notificationProjectKey, allowInsecure: true},
		{name: "loopback IPv4", url: "http://127.0.0.1:8080", key: notificationProjectKey},
		{name: "loopback IPv6", url: "http://[::1]:8080", key: notificationProjectKey},
		{name: "localhost", url: "http://localhost:8080", key: notificationProjectKey},
		{name: "missing URL", key: notificationProjectKey, wantError: "BaseURL is required"},
		{name: "missing project key", url: notificationHTTPSURL, wantError: "ProjectKey is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			client := &http.Client{Transport: notificationRoundTripper(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("constructor must not send requests")
			})}
			manager, err := adminhost.NewNotificationManager(adminhost.NotificationConfig{
				BaseURL: tc.url, ProjectKey: tc.key, AllowInsecureHTTP: tc.allowInsecure, HTTPClient: client,
			})
			if tc.wantError == "" {
				require.NoError(t, err)
				require.NotNil(t, manager)
			} else {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, manager)
			}
			require.Zero(t, calls)
		})
	}
}
