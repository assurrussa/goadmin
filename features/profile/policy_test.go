package profile //nolint:testpackage // Verify private pending-state transitions without exporting test-only APIs.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
)

const (
	testDisplay    = "Display"
	testAvatarHost = "images.example.test"
	testAvatarURL  = "https://images.example.test/avatar.png"
	testNative     = "native-browser-digest"
	testSelector   = "synthetic-profile-selector"
)

func testBinding() host.ExternalBinding {
	now := time.Now().UTC()
	return host.ExternalBinding{
		LocalSubject: goauth.NewSubjectID(), Issuer: "https://issuer.example.test", Subject: "person",
		LinkID: "link-instance", ClientID: "client", ProjectID: "project", AuthorityID: "rp-reference",
		Generation: 1, LoginGeneration: 1, ProviderSessionID: "central-session",
		ExternalAuthTime: now.Add(-time.Minute), Deadline: now.Add(time.Minute), AbsoluteUntil: now.Add(time.Hour),
		Proof: host.ExternalProof{IssuedAt: now, ExpiresAt: now.Add(time.Hour)},
	}
}

func TestAvatarPolicyIsExplicitBoundedAndNetworkFree(t *testing.T) {
	require.NoError(t, ValidateAvatarHosts(nil))
	require.NoError(t, ValidateAvatarHosts([]string{testAvatarHost}))
	for _, host := range []string{
		"localhost", "127.0.0.1", "169.254.169.254", "::1",
		"images.local", "metadata.google.internal", "*.example.test", "Images.example.test",
		"images.example.test:443", "images.example.test/", "-bad.example.test", "images..test",
	} {
		require.ErrorIs(t, ValidateAvatarHosts([]string{host}), ErrInvalid, host)
	}
	require.ErrorIs(t, ValidateAvatarHosts([]string{testAvatarHost, testAvatarHost}), ErrInvalid)
	policy := avatarPolicy{hosts: []string{testAvatarHost}}
	require.True(t, policy.validate(testAvatarURL))
	for _, value := range []string{
		"http://images.example.test/a", "https://images.example.test:443/a",
		"https://other.example.test/a", "https://images.example.test@other.example.test/a",
		"https://images.example.test/a#fragment", "//images.example.test/a",
		"https://images.example.test/%0d", "javascript:alert(1)", "data:image/png;base64,a",
		"https://images.example.test\\other/a",
	} {
		require.False(t, policy.validate(value), value)
	}
	now := time.Now()
	value, err := (avatarPolicy{}).snapshot(Snapshot{AvatarURL: testAvatarURL, IssuedAt: now}, now)
	require.NoError(t, err)
	require.Empty(t, value.AvatarURL, "no configured host must not force remote image loading")
}

func TestPresentationClaimsNeverBecomeAuthorityOrMutableIdentity(t *testing.T) {
	now := time.Now()
	claims := map[string]string{
		displayNameClaim: testDisplay, "avatar_url": testAvatarURL, "login": "forged-login",
		"role": "super_admin", "email_verified": "true", "subject_id": "forged-subject",
	}
	snapshot := FromClaims(claims, "canonical-label", now)
	require.Equal(t, testDisplay, snapshot.Name)
	require.Equal(t, "canonical-label", snapshot.Login)
	claims[displayNameClaim] = "edited after copy"
	require.Equal(t, testDisplay, snapshot.Name)
	policy := avatarPolicy{hosts: []string{testAvatarHost}}
	_, err := policy.snapshot(snapshot, now)
	require.NoError(t, err)
	for _, value := range []Snapshot{
		{Name: strings.Repeat("x", 257), IssuedAt: now},
		{Login: "bad\nlabel", IssuedAt: now},
		{IssuedAt: now.Add(-31 * time.Second)},
		{IssuedAt: now.Add(31 * time.Second)},
		{},
	} {
		_, err = policy.snapshot(value, now)
		require.ErrorIs(t, err, ErrInvalid)
	}
}

func TestPendingIntentExactPinsTTLAndNoReplayAuthority(t *testing.T) {
	now := time.Now()
	binding := testBinding()
	p := pending{
		Selector: digest(testSelector), Native: testNative, Started: now,
		Until: now.Add(refreshLifetime), Original: binding,
	}
	require.True(t, p.valid(testSelector, testNative, now))
	require.False(t, p.valid("another-selector", testNative, now))
	require.False(t, p.valid(testSelector, "another-browser", now))
	require.False(t, p.valid(testSelector, testNative, p.Until))
	p.Until = now.Add(refreshLifetime + time.Second)
	require.False(t, p.valid(testSelector, testNative, now))
	require.True(t, samePins(binding, binding))
	for _, edit := range []func(*host.ExternalBinding){
		func(b *host.ExternalBinding) { b.LinkID = "relinked-instance" },
		func(b *host.ExternalBinding) { b.Subject = "another-person" },
		func(b *host.ExternalBinding) { b.LocalSubject = goauth.NewSubjectID() },
		func(b *host.ExternalBinding) { b.Issuer = "https://foreign.example.test" },
		func(b *host.ExternalBinding) { b.ProjectID = "another-project" },
		func(b *host.ExternalBinding) { b.ClientID = "another-client" },
	} {
		other := binding
		edit(&other)
		require.False(t, samePins(binding, other))
	}
	other := binding
	other.LoginGeneration++
	require.False(t, sameProof(binding, other))
	other = binding
	other.AuthorityID = "other-rp-reference"
	require.False(t, sameProof(binding, other))
}

func TestRefreshCookieRejectsDuplicateWithoutAcceptingIdentityJSON(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://app.example.test/callback", nil)
	request.AddCookie(&http.Cookie{
		Name: refreshCookie, Value: testSelector, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	value, err := RequestSelector(request)
	require.NoError(t, err)
	require.Equal(t, testSelector, value)
	request.AddCookie(&http.Cookie{
		Name: refreshCookie, Value: "other", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	_, err = RequestSelector(request)
	require.ErrorIs(t, err, ErrDenied)
}

func TestSelectionVersionNeverWrapsOrAcceptsApproximateJSONNumber(t *testing.T) {
	require.NoError(t, nextVersion(record{Version: 1}, "1"))
	require.ErrorIs(t, nextVersion(record{Version: 1}, "1.0"), ErrConflict)
	require.ErrorIs(t, nextVersion(record{Version: 1}, "2"), ErrConflict)
	require.ErrorIs(t, nextVersion(record{Version: 9223372036854775807}, "9223372036854775807"), ErrConflict)
}

func TestCompletionClaimCookieIsOpaqueUniqueAndBounded(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "https://app.example.test/profile-source/complete", nil)
	value, err := uniqueCookie(request, completionCookie)
	require.NoError(t, err)
	require.Empty(t, value)
	request.AddCookie(&http.Cookie{
		Name: completionCookie, Value: "synthetic-completion-claim", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	value, err = uniqueCookie(request, completionCookie)
	require.NoError(t, err)
	require.NotEmpty(t, value)
	request.AddCookie(&http.Cookie{
		Name: completionCookie, Value: "duplicate", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	_, err = uniqueCookie(request, completionCookie)
	require.ErrorIs(t, err, ErrDenied)
}
