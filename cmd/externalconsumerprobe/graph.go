package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/assurrussa/goadmin/internal/externalconsumerprobe"
)

func downloadGraph(ctx context.Context, dir, cache string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "mod", "download", "all")
	cmd.Dir, cmd.Env = dir, commandEnv(cache)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("download declared consumer graph: %w: %s", err, out)
	}
	return nil
}

func verifyGraph(ctx context.Context, dir, cache string, timeout time.Duration, cfg externalconsumerprobe.Config) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-mod=mod", "-m", "-json", "all")
	cmd.Dir, cmd.Env = dir, commandEnv(cache)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("inspect consumer graph: %w", err)
	}
	expected := map[string]string{}
	for module, path := range map[string]string{
		cfg.ModulePath:                      cfg.LocalPath,
		cfg.GoauthModulePath:                cfg.GoauthLocalPath,
		"github.com/assurrussa/gonotify":    cfg.GonotifyLocalPath,
		"github.com/assurrussa/gouploads":   cfg.GouploadsLocalPath,
		"github.com/assurrussa/gowebsocket": cfg.GowebsocketLocalPath,
	} {
		if path != "" {
			absolute, absErr := filepath.Abs(path)
			if absErr != nil {
				return absErr
			}
			expected[module] = absolute
		}
	}
	return checkGraph(bytes.NewReader(out), expected)
}

// checkGraph rejects private legacy modules and any unrequested source substitution.
func checkGraph(input io.Reader, expected map[string]string) error {
	seen := map[string]bool{}
	decoder := json.NewDecoder(input)
	for {
		var module selectedModule
		if err := decoder.Decode(&module); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("decode module graph: %w", err)
		}
		if module.Path == "github.com/assurrussa/goshared" || module.Path == "github.com/assurrussa/goredis" ||
			module.Path == "github.com/assurrussa/gofiber" {
			return fmt.Errorf("forbidden legacy module in graph: %s", module.Path)
		}
		path, local := expected[module.Path]
		if module.Replace != nil {
			if !local || filepath.Clean(module.Replace.Path) != filepath.Clean(path) {
				return fmt.Errorf("unexpected replacement: %s => %s", module.Path, module.Replace.Path)
			}
			seen[module.Path] = true
		} else if local {
			return fmt.Errorf("missing replacement for candidate %s", module.Path)
		}
	}
	for module := range expected {
		if !seen[module] {
			return fmt.Errorf("candidate missing from graph: %s", module)
		}
	}
	return nil
}
