package host //nolint:testpackage // exercises secret-writing failure paths without exporting test seams

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSetupSecretOutputRejectsStandardStreamsAndLooseFiles(t *testing.T) {
	t.Parallel()

	_, err := ValidateSetupSecretOutput(os.Stdout)
	require.Error(t, err)

	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, nil, 0o644)) //nolint:gosec // deliberately unsafe mode must be rejected
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	// Creation permissions are filtered by umask; explicitly establish the unsafe fixture.
	require.NoError(t, file.Chmod(0o644)) //nolint:gosec // deliberately unsafe mode must be rejected
	info, err := file.Stat()
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	_, err = ValidateSetupSecretOutput(file)
	require.Error(t, err)
}

func TestValidateSetupSecretOutputAcceptsMode0600File(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	kind, err := ValidateSetupSecretOutput(file)
	require.NoError(t, err)
	require.Equal(t, SetupSecretOutputFile, kind)
}

func TestIssueFirstAdminSetupTokenWritesSecretOnlyToSecretChannel(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("t", 43)
	issuer := &setupIssuerStub{token: token}
	secretOutput := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	logs := &bytes.Buffer{}

	result, err := issueFirstAdminSetupToken(
		context.Background(),
		issuer,
		"https://admin.example.test/auth/register",
		secretOutput,
		SetupSecretOutputFile,
	)
	require.NoError(t, err)
	require.Equal(t, "https://admin.example.test/auth/register", result.SetupURL)
	require.Equal(t, token+"\n", secretOutput.String())
	require.NotContains(t, stdout.String(), token)
	require.NotContains(t, stderr.String(), token)
	require.NotContains(t, logs.String(), token)
}

func TestIssueFirstAdminSetupTokenWritesFullURLOnlyToTTY(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("t", 43)
	issuer := &setupIssuerStub{token: token}
	secretOutput := &bytes.Buffer{}

	_, err := issueFirstAdminSetupToken(
		context.Background(),
		issuer,
		"https://admin.example.test/auth/register?source=platformctl",
		secretOutput,
		SetupSecretOutputTTY,
	)
	require.NoError(t, err)
	require.Contains(t, secretOutput.String(), "#setup_token="+token)
	require.Contains(t, secretOutput.String(), "source=platformctl")
	require.NotContains(t, secretOutput.String(), "?setup_token=")
}

func TestIssueFirstAdminSetupTokenRevokesSecretWhenOutputFails(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("t", 43)
	issuer := &setupIssuerStub{token: token}
	_, err := issueFirstAdminSetupToken(
		context.Background(),
		issuer,
		"https://admin.example.test/auth/register",
		errorWriter{},
		SetupSecretOutputFile,
	)
	require.Error(t, err)
	require.Equal(t, []string{token}, issuer.revoked)
}

func TestValidateFirstAdminSetupURLRejectsCredentialAndTokenLeakage(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://user:password@admin.example.test/auth/register",
		"https://admin.example.test/auth/register?setup_token=secret",
		"https://admin.example.test/auth/register#secret",
		"/auth/register",
	} {
		_, err := validateFirstAdminSetupURL(raw)
		require.Error(t, err, raw)
	}
}

type setupIssuerStub struct {
	token   string
	revoked []string
}

func (s *setupIssuerStub) Issue(context.Context) (string, error) {
	return s.token, nil
}

func (s *setupIssuerStub) Revoke(_ context.Context, token string) error {
	s.revoked = append(s.revoked, token)

	return nil
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestFirstAdminSetupTokenConfigDoesNotExposeTokenFields(t *testing.T) {
	t.Parallel()

	configDump := strings.ToLower("FirstAdminSetupTokenConfig{SetupURL, TTL, SecretOutput}")
	require.NotContains(t, configDump, "rawtoken")
	require.NotContains(t, configDump, "tokenvalue")
}
