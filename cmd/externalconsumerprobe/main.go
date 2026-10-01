package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	externalconsumerprobe "github.com/assurrussa/goadmin/internal/externalconsumerprobe"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("externalconsumerprobe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	modulePath := fs.String("module", externalconsumerprobe.DefaultModulePath, "target Go module path")
	version := fs.String("version", "", "published target version to resolve and require")
	localPath := fs.String("local-path", "", "local checkout path for replace-based probe")
	goauthModulePath := fs.String("goauth-module", externalconsumerprobe.DefaultGoauthModulePath, "goauth Go module path")
	goauthVersion := fs.String("goauth-version", "", "published goauth version to resolve and require")
	goauthLocalPath := fs.String("goauth-local-path", "", "local goauth checkout path for replace-based probe")
	gouploadsLocalPath := fs.String("gouploads-local-path", "", "local gouploads checkout for a local probe")
	gowebsocketLocalPath := fs.String("gowebsocket-local-path", "", "local gowebsocket checkout for a local probe")
	gonotifyLocalPath := fs.String("gonotify-local-path", "", "local gonotify checkout for a local probe")
	goModCache := fs.String("go-mod-cache", "", "optional GOMODCACHE path for the probe commands")
	keepWorkdir := fs.Bool("keep-workdir", false, "keep the generated temporary probe module on disk")
	timeout := fs.Duration("timeout", 2*time.Minute, "timeout for each go command")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := externalconsumerprobe.Config{
		ModulePath:           *modulePath,
		Version:              *version,
		LocalPath:            *localPath,
		GoauthModulePath:     *goauthModulePath,
		GoauthVersion:        *goauthVersion,
		GoauthLocalPath:      *goauthLocalPath,
		GouploadsLocalPath:   *gouploadsLocalPath,
		GonotifyLocalPath:    *gonotifyLocalPath,
		GowebsocketLocalPath: *gowebsocketLocalPath,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	if cfg.Version != "" {
		if err := goListModule(ctx, cfg.ModulePath, cfg.Version, *goModCache, *timeout); err != nil {
			return err
		}
	}
	if cfg.GoauthVersion != "" {
		if err := goListModule(ctx, cfg.GoauthModulePath, cfg.GoauthVersion, *goModCache, *timeout); err != nil {
			return err
		}
	}

	workdir, err := os.MkdirTemp("", "goadmin-externalconsumerprobe-*")
	if err != nil {
		return fmt.Errorf("create probe workdir: %w", err)
	}
	if !*keepWorkdir {
		defer func() {
			_ = os.RemoveAll(workdir)
		}()
	}

	goMod, err := cfg.BuildGoMod()
	if err != nil {
		return err
	}
	testFile, err := cfg.BuildProbeTest()
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(workdir, "go.mod"), []byte(goMod), 0o644); err != nil { //nolint:gosec // required
		return fmt.Errorf("write probe go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "externalconsumer_probe_test.go"), []byte(testFile), 0o644); err != nil { //nolint:gosec,lll // required
		return fmt.Errorf("write probe test: %w", err)
	}

	if err := downloadGraph(ctx, workdir, *goModCache, *timeout); err != nil {
		return err
	}
	if err := verifyGraph(ctx, workdir, *goModCache, *timeout, cfg); err != nil {
		return err
	}
	if err := goTestProbe(ctx, workdir, *goModCache, *timeout); err != nil {
		if *keepWorkdir {
			return fmt.Errorf("%w (workdir preserved at %s)", err, workdir)
		}
		return fmt.Errorf("%w (rerun with --keep-workdir to inspect generated probe module)", err)
	}
	if cfg.Version != "" {
		if err := verifySelectedModuleVersion(ctx, workdir, *goModCache, *timeout, cfg.ModulePath, cfg.Version); err != nil {
			return err
		}
	}
	if cfg.GoauthVersion != "" {
		if err := verifySelectedModuleVersion(
			ctx, workdir, *goModCache, *timeout, cfg.GoauthModulePath, cfg.GoauthVersion,
		); err != nil {
			return err
		}
	}

	if err := verifyGraph(ctx, workdir, *goModCache, *timeout, cfg); err != nil {
		return err
	}
	if *keepWorkdir {
		_, _ = fmt.Fprintln(os.Stdout, workdir)
	}

	return nil
}

func goListModule(ctx context.Context, modulePath string, version string, goModCache string, timeout time.Duration) error {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, "go", "list", "-m", "-json", modulePath+"@"+version) //nolint:gosec // required
	cmd.Env = commandEnv(goModCache)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("resolve published module %s@%s: timeout after %s", modulePath, version, timeout)
		}
		return fmt.Errorf("resolve published module %s@%s: %w", modulePath, version, err)
	}

	return nil
}

func goTestProbe(ctx context.Context, workdir string, goModCache string, timeout time.Duration) error {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, "go", probeTestArgs()...) //nolint:gosec // required
	cmd.Dir = workdir
	cmd.Env = commandEnv(goModCache)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("run external consumer probe: timeout after %s", timeout)
		}
		return fmt.Errorf("run external consumer probe: %w", err)
	}

	return nil
}

type selectedModule struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Replace *struct {
		Path string `json:"path"`
	} `json:"replace"`
}

func verifySelectedModuleVersion(
	ctx context.Context,
	workdir, goModCache string,
	timeout time.Duration,
	modulePath, expected string,
) error {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx, "go", "list", "-mod=mod", "-m", "-json", modulePath,
	)
	cmd.Dir = workdir
	cmd.Env = commandEnv(goModCache)
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("inspect selected module %s: timeout after %s", modulePath, timeout)
		}
		return fmt.Errorf("inspect selected module %s: %w", modulePath, err)
	}

	var selected selectedModule
	if err := json.Unmarshal(output, &selected); err != nil {
		return fmt.Errorf("decode selected module %s: %w", modulePath, err)
	}
	return checkSelectedModuleVersion(modulePath, expected, selected)
}

func checkSelectedModuleVersion(modulePath, expected string, selected selectedModule) error {
	if selected.Path != modulePath || selected.Version != expected || selected.Replace != nil {
		return fmt.Errorf("external consumer selected %s@%s (replace=%v), expected published %s@%s", selected.Path, selected.Version, selected.Replace != nil, modulePath, expected) //nolint:lll // includes both selected and required module identities
	}
	return nil
}

func probeTestArgs() []string {
	return []string{"test", "-mod=mod", "./...", "-count=1"}
}

func commandEnv(goModCache string) []string {
	env := os.Environ()
	if goModCache == "" {
		return env
	}

	return append(env, "GOMODCACHE="+goModCache)
}
