package adminservice //nolint:testpackage // verifies canonical profile operations with the real runtime.

import (
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/stretchr/testify/require"
)

func TestProfilePendingEmailChangeExpiresCanonically(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	fixture, err := testkit.NewRuntime(func(config *goauth.Config) { config.Now = func() time.Time { return now } })
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "current@example.test", Password: testAdminPassword,
	})
	require.NoError(t, err)
	service := NewService(nil, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{}))
	pending, err := service.GetPendingEmailChange(t.Context(), account.Subject.ID)
	require.Nil(t, pending)
	require.ErrorIs(t, err, goauth.ErrEmailChangeNotFound)
	err = service.RequestEmailChangeWithPassword(t.Context(), account.Subject.ID, testAdminPassword, "next@example.test")
	require.NoError(t, err)
	pending, err = service.GetPendingEmailChange(t.Context(), account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, "next@example.test", pending.NewDisplayValue)
	now = pending.ExpiresAt.Add(time.Second)
	pending, err = service.GetPendingEmailChange(t.Context(), account.Subject.ID)
	require.Nil(t, pending)
	require.ErrorIs(t, err, goauth.ErrEmailChangeNotFound)
	current, err := fixture.Runtime.GetAccount(t.Context(), account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, "current@example.test", current.PrimaryEmail.DisplayValue)
}

func TestProfileSettingsPreserveLocalAndLinkedSSOAccounts(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "linked SSO"}[linked], func(t *testing.T) {
			fixture, err := testkit.NewRuntime()
			require.NoError(t, err)
			account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
				Email: "current@example.test", Password: testAdminPassword,
			})
			require.NoError(t, err)
			loginRequest := goauth.LoginRequest{
				Credential: goauth.Credential{
					Identifier: goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: account.PrimaryEmail.DisplayValue},
					Password:   testAdminPassword,
				},
				Realm: goauth.RealmAdmin,
			}
			external := goauth.ExternalIdentity{
				Issuer: "https://idp.example.test", Subject: "linked-admin",
				Email: account.PrimaryEmail.DisplayValue, EmailVerified: true,
			}
			if linked {
				login, loginErr := fixture.Runtime.Login(t.Context(), loginRequest)
				require.NoError(t, loginErr)
				auth, verifyErr := fixture.Runtime.AuthenticateSession(t.Context(), login.Tokens.AccessToken)
				require.NoError(t, verifyErr)
				_, linkErr := fixture.Runtime.LinkExternalIdentity(t.Context(), auth, external)
				require.NoError(t, linkErr)
			}
			service := NewService(nil, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{}))
			err = service.RequestEmailChangeWithPassword(t.Context(), account.Subject.ID, "incorrect", "next@example.test")
			require.ErrorIs(t, err, goauth.ErrCurrentPasswordInvalid)
			require.NoError(t, service.RequestEmailChangeWithPassword(
				t.Context(), account.Subject.ID, testAdminPassword, "next@example.test",
			))
			pending, pendingErr := service.GetPendingEmailChange(t.Context(), account.Subject.ID)
			require.NoError(t, pendingErr)
			require.Equal(t, "next@example.test", pending.NewDisplayValue)
			const nextPassword = "New-Unique-Admin-Passphrase-987"
			err = service.ChangePassword(t.Context(), account.Subject.ID, "incorrect", nextPassword)
			require.ErrorIs(t, err, goauth.ErrCurrentPasswordInvalid)
			_, err = fixture.Runtime.Login(t.Context(), loginRequest)
			require.NoError(t, err)
			require.NoError(t, service.ChangePassword(t.Context(), account.Subject.ID, testAdminPassword, nextPassword))
			_, err = fixture.Runtime.Login(t.Context(), loginRequest)
			require.Error(t, err)
			loginRequest.Credential.Password = nextPassword
			login, err := fixture.Runtime.Login(t.Context(), loginRequest)
			require.NoError(t, err)
			require.Equal(t, account.Subject.ID, login.Account.Subject.ID)
			if linked {
				current, resolveErr := fixture.Runtime.ResolveExternalIdentity(t.Context(), external)
				require.NoError(t, resolveErr)
				require.Equal(t, account.Subject.ID, current.Subject.ID)
			}
		})
	}
}
