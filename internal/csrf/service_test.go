package csrf_test

import (
	"context"
	"strings"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/internal/identity"
)

const (
	testProofSecret = "synthetic-csrf-secret-at-least-32-bytes" //nolint:gosec // synthetic csrf test secret
	testAdminOrigin = "https://admin.test"
)

func TestPurposeBoundProofWireFormatAndBinding(t *testing.T) {
	service, err := csrf.New(csrf.NewOptions("admin.test", testProofSecret, []string{testAdminOrigin}, logger.Discard()))
	require.NoError(t, err)
	id := identity.NewUserID()
	req := csrf.Request{SessionID: "session-digest", UserID: id}
	token, err := service.Create(context.Background(), req)
	require.NoError(t, err)
	claims, valid, err := service.Check(context.Background(), req, token.Token)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, id, claims.UserID)
	require.Equal(t, "browser-csrf", claims.Purpose)
	for _, invalid := range []csrf.Request{
		{SessionID: "other-session", UserID: id},
		{SessionID: req.SessionID, UserID: identity.NewUserID()},
		{SessionID: req.SessionID},
	} {
		_, valid, err := service.Check(context.Background(), invalid, token.Token)
		require.Error(t, err)
		require.False(t, valid)
	}
}

func TestRejectProofsOutsideCSRFProfile(t *testing.T) {
	service, err := csrf.New(csrf.NewOptions("admin.test", testProofSecret, []string{testAdminOrigin}, logger.Discard()))
	require.NoError(t, err)
	id := identity.NewUserID()
	req := csrf.Request{SessionID: "session-digest", UserID: id}
	for _, mode := range []string{
		"legacy", "no-exp", "expired", "no-iat", "future-iat",
		"wrong-issuer", "wrong-audience", "wrong-purpose", "HS512",
	} {
		t.Run(mode, func(t *testing.T) {
			claims := jwt.MapClaims{
				"sid": req.SessionID, "sub": uuid.UUID(id).String(), "purpose": "browser-csrf",
				"iss": "goadmin-csrf-v1", "aud": "admin.test",
				"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			}
			var method jwt.SigningMethod = jwt.SigningMethodHS256
			switch mode {
			case "legacy":
				delete(claims, "purpose")
				delete(claims, "iss")
				delete(claims, "aud")
			case "no-exp":
				delete(claims, "exp")
			case "expired":
				claims["exp"] = time.Now().Add(-time.Minute).Unix()
			case "no-iat":
				delete(claims, "iat")
			case "future-iat":
				claims["iat"] = time.Now().Add(time.Hour).Unix()
			case "wrong-issuer":
				claims["iss"] = "another-service"
			case "wrong-audience":
				claims["aud"] = "another.test"
			case "wrong-purpose":
				claims["purpose"] = "password-reset"
			case "HS512":
				method = jwt.SigningMethodHS512
			}
			token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(testProofSecret))
			require.NoError(t, err)
			_, valid, err := service.Check(context.Background(), req, token)
			require.Error(t, err)
			require.False(t, valid)
		})
	}
	for _, input := range []string{"invalid", "", strings.Repeat("x", 8193)} {
		_, valid, err := service.Check(context.Background(), req, input)
		require.Error(t, err)
		require.False(t, valid)
	}
}

func TestRejectNonPositiveProofLifetime(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Hour} {
		opts := csrf.NewOptions("admin.test", testProofSecret, []string{testAdminOrigin}, logger.Discard(), csrf.WithTokenTTL(ttl))
		_, err := csrf.New(opts)
		require.Error(t, err)
	}
}
