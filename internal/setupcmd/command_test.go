package setupcmd //nolint:testpackage // validates command hooks before database connection and token creation

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

func TestRunNonInteractiveWritesRawTokenOnlyToSecretFile(t *testing.T) {
	t.Parallel()

	rawToken := strings.Repeat("t", 43)
	secretPath := filepath.Join(t.TempDir(), "setup.secret")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	issueCalls := 0

	exitCode := run(
		context.Background(),
		[]string{
			"--non-interactive",
			"--secret-file", secretPath,
			"--setup-url", "https://admin.example.test/auth/register",
		},
		stdout,
		stderr,
		dependencies{
			getenv: func(key string) string {
				if key == databaseDSNEnv {
					return "postgres://configured-outside-command"
				}
				return ""
			},
			issue: func(_ context.Context, input issueInput) (adminhost.FirstAdminSetupTokenResult, error) {
				issueCalls++
				_, err := fmt.Fprintln(input.output, rawToken)
				return adminhost.FirstAdminSetupTokenResult{SetupURL: input.setupURL}, err
			},
		},
	)

	require.Zero(t, exitCode)
	require.Equal(t, 1, issueCalls)
	secret, err := os.ReadFile(secretPath)
	require.NoError(t, err)
	require.Equal(t, rawToken+"\n", string(secret))
	info, err := os.Stat(secretPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	require.Contains(t, stdout.String(), "https://admin.example.test/auth/register")
	require.NotContains(t, stdout.String(), rawToken)
	require.NotContains(t, stderr.String(), rawToken)
}

func TestRunNonInteractiveWithoutSecretChannelStopsBeforeIssue(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	issueCalls := 0
	exitCode := run(
		context.Background(),
		[]string{"--non-interactive", "--setup-url", "https://admin.example.test/auth/register"},
		stdout,
		stderr,
		dependencies{
			getenv: func(string) string { return "configured" },
			issue: func(context.Context, issueInput) (adminhost.FirstAdminSetupTokenResult, error) {
				issueCalls++
				return adminhost.FirstAdminSetupTokenResult{}, nil
			},
		},
	)

	require.Equal(t, 1, exitCode)
	require.Zero(t, issueCalls)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "requires --secret-file or --secret-fd")
}

func TestOpenSecretFileRejectsSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))

	_, err := openSecretFile(link)
	require.Error(t, err)
}
