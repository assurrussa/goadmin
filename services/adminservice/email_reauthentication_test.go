package adminservice //nolint:testpackage // exercises the runtime boundary using canonical fixtures.

import (
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/stretchr/testify/require"
)

func TestEmailRequestReauthenticatesCanonicalSubject(t *testing.T) {
	for _, verified := range []bool{false, true} {
		name := "unverified"
		if verified {
			name = "verified"
		}
		t.Run(name, func(t *testing.T) {
			fixture, err := testkit.NewRuntime()
			require.NoError(t, err)
			request := goauth.RegisterRequest{Email: "previous@example.test", Password: testAdminPassword}
			var account goauth.Account
			if verified {
				account, err = fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), request)
			} else {
				result, registerErr := fixture.Runtime.Register(t.Context(), request)
				err, account = registerErr, result.Account
			}
			require.NoError(t, err)
			service := NewService(nil, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{}))
			baselineEvents := len(fixture.Events.Events())
			const otherPassword = "Different-Subject-Passphrase-42" //nolint:gosec // isolated fixture credential.
			_, err = fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
				Email: "other@example.test", Password: otherPassword,
			})
			require.NoError(t, err)
			for _, password := range []string{"", "wrong-password", otherPassword} {
				err = service.RequestEmailChangeWithPassword(t.Context(), account.Subject.ID, password, "next@example.test")
				require.ErrorIs(t, err, goauth.ErrCurrentPasswordInvalid)
				_, err = fixture.Runtime.PendingEmailChange(t.Context(), account.Subject.ID)
				require.ErrorIs(t, err, goauth.ErrEmailChangeNotFound)
				require.Len(t, fixture.Events.Events(), baselineEvents)
			}
			err = service.RequestEmailChangeWithPassword(t.Context(), account.Subject.ID, testAdminPassword, "next@example.test")
			require.NoError(t, err)
			pending, err := fixture.Runtime.PendingEmailChange(t.Context(), account.Subject.ID)
			require.NoError(t, err)
			require.Equal(t, "next@example.test", pending.NewDisplayValue)
			require.Len(t, fixture.Events.Events(), baselineEvents+1)
		})
	}
}

func TestEmailRequestRejectsSSOOnlyAccount(t *testing.T) {
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	account, err := fixture.Runtime.ResolveExternalIdentity(t.Context(), goauth.ExternalIdentity{
		Issuer: "https://idp.example.test", Subject: "sso-only", Email: "sso@example.test", EmailVerified: true,
	})
	require.NoError(t, err)
	service := NewService(nil, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{}))
	err = service.RequestEmailChangeWithPassword(t.Context(), account.Subject.ID, testAdminPassword, "next@example.test")
	require.ErrorIs(t, err, goauth.ErrCurrentPasswordInvalid)
	_, err = fixture.Runtime.PendingEmailChange(t.Context(), account.Subject.ID)
	require.ErrorIs(t, err, goauth.ErrEmailChangeNotFound)
	require.Empty(t, fixture.Events.Events())
}
