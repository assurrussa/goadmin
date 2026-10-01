package setupcmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	adminhost "github.com/assurrussa/goadmin/host"
)

const (
	databaseDSNEnv = "GOADMIN_DATABASE_DSN"
	setupURLEnv    = "GOADMIN_SETUP_URL"
)

type issueInput struct {
	databaseDSN string
	setupURL    string
	ttl         time.Duration
	output      *os.File
}

type issueFunc func(context.Context, issueInput) (adminhost.FirstAdminSetupTokenResult, error)

type dependencies struct {
	getenv  func(string) string
	openTTY func() (*os.File, error)
	issue   issueFunc
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return run(ctx, args, stdout, stderr, dependencies{
		getenv: os.Getenv,
		openTTY: func() (*os.File, error) {
			return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		},
		issue: issue,
	})
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("goadmin-setup-token", flag.ContinueOnError)
	flags.SetOutput(stderr)
	setupURL := flags.String("setup-url", "", "absolute first-admin registration URL")
	ttl := flags.Duration("ttl", 15*time.Minute, "setup token lifetime")
	nonInteractive := flags.Bool("non-interactive", false, "require a secret file or descriptor")
	secretFile := flags.String("secret-file", "", "0600 file that receives the raw token")
	secretFD := flags.String("secret-fd", "", "pre-opened 0600 descriptor that receives the raw token")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "goadmin-setup-token does not accept positional arguments")
		return 2
	}
	if *ttl < time.Minute || *ttl > time.Hour {
		_, _ = fmt.Fprintln(stderr, "setup token ttl must be between 1 minute and 1 hour")
		return 2
	}

	output, err := openSecretOutput(*nonInteractive, *secretFile, *secretFD, deps.openTTY)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "open setup secret channel: %v\n", err)
		return 1
	}
	defer func() { _ = output.Close() }()

	kind, err := adminhost.ValidateSetupSecretOutput(output)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "validate setup secret channel: %v\n", err)
		return 1
	}
	if !*nonInteractive && kind != adminhost.SetupSecretOutputTTY {
		_, _ = fmt.Fprintln(stderr, "interactive setup requires /dev/tty")
		return 1
	}

	resolvedURL := strings.TrimSpace(*setupURL)
	if resolvedURL == "" {
		resolvedURL = strings.TrimSpace(deps.getenv(setupURLEnv))
	}
	if resolvedURL == "" {
		_, _ = fmt.Fprintln(stderr, "setup URL is required through --setup-url or GOADMIN_SETUP_URL")
		return 2
	}
	dsn := strings.TrimSpace(deps.getenv(databaseDSNEnv))
	if dsn == "" {
		_, _ = fmt.Fprintln(stderr, "GOADMIN_DATABASE_DSN is required")
		return 2
	}

	result, err := deps.issue(ctx, issueInput{
		databaseDSN: dsn,
		setupURL:    resolvedURL,
		ttl:         *ttl,
		output:      output,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "issue first admin setup token: %v\n", err)
		return 1
	}
	if *nonInteractive {
		_, _ = fmt.Fprintf(stdout, "Setup page: %s\n", result.SetupURL)
	}

	return 0
}

func issue(ctx context.Context, input issueInput) (adminhost.FirstAdminSetupTokenResult, error) {
	db, err := adminhost.NewPgsqlClient(ctx, adminhost.PgsqlConfig{DSN: input.databaseDSN}, adminhost.DiscardLogger())
	if err != nil {
		return adminhost.FirstAdminSetupTokenResult{}, err
	}
	defer func() { _ = db.Close() }()

	return adminhost.IssueFirstAdminSetupToken(ctx, db, adminhost.NewTxManager(db), adminhost.FirstAdminSetupTokenConfig{
		SetupURL:     input.setupURL,
		TTL:          input.ttl,
		SecretOutput: input.output,
	})
}

func openSecretOutput(
	nonInteractive bool,
	secretPath string,
	secretFD string,
	openTTY func() (*os.File, error),
) (*os.File, error) {
	secretPath = strings.TrimSpace(secretPath)
	secretFD = strings.TrimSpace(secretFD)
	if secretPath != "" && secretFD != "" {
		return nil, errors.New("choose exactly one of --secret-file or --secret-fd")
	}
	if !nonInteractive {
		if secretPath != "" || secretFD != "" {
			return nil, errors.New("secret file and descriptor require --non-interactive")
		}

		return openTTY()
	}
	if secretPath == "" && secretFD == "" {
		return nil, errors.New("non-interactive setup requires --secret-file or --secret-fd")
	}
	if secretFD != "" {
		fd, err := strconv.Atoi(secretFD)
		if err != nil || fd <= int(os.Stderr.Fd()) {
			return nil, errors.New("secret descriptor must be an integer greater than 2")
		}

		return os.NewFile(uintptr(fd), "goadmin-setup-secret-fd"), nil
	}

	return openSecretFile(secretPath)
}

func openSecretFile(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 {
		return nil, errors.New("existing setup secret path must be a regular file with mode 0600")
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !os.SameFile(before, after) {
		_ = file.Close()
		return nil, errors.New("setup secret path changed while opening")
	}

	return file, nil
}
