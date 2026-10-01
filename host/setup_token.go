package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/assurrussa/goadmin/internal/firstadminsetup"
)

const FirstAdminSetupTokenKey = "setup_token"

type SetupSecretOutputKind string

const (
	SetupSecretOutputTTY  SetupSecretOutputKind = "tty"
	SetupSecretOutputFile SetupSecretOutputKind = "file"
	SetupSecretOutputPipe SetupSecretOutputKind = "pipe"
)

type FirstAdminSetupTokenConfig struct {
	SetupURL     string
	TTL          time.Duration
	SecretOutput *os.File
}

type FirstAdminSetupTokenResult struct {
	SetupURL string
}

type firstAdminSetupTokenIssuer interface {
	Issue(ctx context.Context) (string, error)
	Revoke(ctx context.Context, token string) error
}

// ValidateSetupSecretOutput rejects stdout/stderr and any output channel whose
// filesystem permissions could disclose a first-admin setup token.
func ValidateSetupSecretOutput(output *os.File) (SetupSecretOutputKind, error) {
	if output == nil {
		return "", errors.New("first admin setup secret output is required")
	}
	if output.Fd() <= os.Stderr.Fd() {
		return "", errors.New("stdin, stdout, and stderr are not setup secret channels")
	}

	info, err := output.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect first admin setup secret output: %w", err)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		if output.Name() != "/dev/tty" {
			return "", errors.New("only /dev/tty is accepted as an interactive setup secret channel")
		}

		return SetupSecretOutputTTY, nil
	}
	if info.Mode().Perm() != 0o600 {
		return "", errors.New("first admin setup secret file or descriptor must have mode 0600")
	}
	if info.Mode().IsRegular() {
		return SetupSecretOutputFile, nil
	}
	if info.Mode()&os.ModeNamedPipe != 0 {
		return SetupSecretOutputPipe, nil
	}

	return "", errors.New("first admin setup secret output must be /dev/tty, a regular file, or a pipe")
}

// IssueFirstAdminSetupToken creates a short-lived one-time token and writes
// the secret only to a prevalidated operator-controlled output channel.
func IssueFirstAdminSetupToken(
	ctx context.Context,
	db StoragePgsqlClient,
	tx StoragePgsqlTxManager,
	cfg FirstAdminSetupTokenConfig,
) (FirstAdminSetupTokenResult, error) {
	kind, err := ValidateSetupSecretOutput(cfg.SecretOutput)
	if err != nil {
		return FirstAdminSetupTokenResult{}, err
	}
	setupURL, err := validateFirstAdminSetupURL(cfg.SetupURL)
	if err != nil {
		return FirstAdminSetupTokenResult{}, err
	}

	store, err := firstadminsetup.NewPostgresStore(db, tx)
	if err != nil {
		return FirstAdminSetupTokenResult{}, err
	}
	options := make([]firstadminsetup.Option, 0, 1)
	if cfg.TTL != 0 {
		options = append(options, firstadminsetup.WithTTL(cfg.TTL))
	}
	issuer, err := firstadminsetup.New(store, options...)
	if err != nil {
		return FirstAdminSetupTokenResult{}, err
	}

	return issueFirstAdminSetupToken(ctx, issuer, setupURL, cfg.SecretOutput, kind)
}

func issueFirstAdminSetupToken(
	ctx context.Context,
	issuer firstAdminSetupTokenIssuer,
	setupURL string,
	output io.Writer,
	kind SetupSecretOutputKind,
) (FirstAdminSetupTokenResult, error) {
	token, err := issuer.Issue(ctx)
	if err != nil {
		return FirstAdminSetupTokenResult{}, err
	}

	secret := token
	if kind == SetupSecretOutputTTY {
		secret, err = firstAdminSetupURL(setupURL, token)
		if err != nil {
			_ = revokeFirstAdminSetupToken(ctx, issuer, token)

			return FirstAdminSetupTokenResult{}, err
		}
	}
	if _, err := io.WriteString(output, secret+"\n"); err != nil {
		revokeErr := revokeFirstAdminSetupToken(ctx, issuer, token)
		if revokeErr != nil {
			return FirstAdminSetupTokenResult{}, errors.Join(
				fmt.Errorf("write first admin setup secret: %w", err),
				fmt.Errorf("revoke unwritten first admin setup token: %w", revokeErr),
			)
		}

		return FirstAdminSetupTokenResult{}, fmt.Errorf("write first admin setup secret: %w", err)
	}

	return FirstAdminSetupTokenResult{SetupURL: setupURL}, nil
}

func revokeFirstAdminSetupToken(
	ctx context.Context,
	issuer firstAdminSetupTokenIssuer,
	token string,
) error {
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	return issuer.Revoke(revokeCtx, token)
}

func validateFirstAdminSetupURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse first admin setup url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("first admin setup url must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("first admin setup url must be absolute and contain no credentials or fragment")
	}
	if parsed.Query().Has(FirstAdminSetupTokenKey) {
		return "", errors.New("first admin setup url must not contain a setup token")
	}

	return parsed.String(), nil
}

func firstAdminSetupURL(setupURL string, token string) (string, error) {
	parsed, err := url.Parse(setupURL)
	if err != nil {
		return "", err
	}
	fragment := url.Values{}
	fragment.Set(FirstAdminSetupTokenKey, token)
	parsed.Fragment = fragment.Encode()

	return parsed.String(), nil
}
