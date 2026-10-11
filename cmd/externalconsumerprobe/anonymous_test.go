package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	consumerModePublished        = "published"
	consumerModeLocal            = "local"
	consumerModeSharedLocal      = "shared-local"
	consumerModeSharedCandidates = "shared-candidates"
	consumerModePublicDeps       = "public-deps-local"
)

func consumerScriptFixture(t *testing.T) (script, stubDir string, env []string) {
	t.Helper()
	dir := t.TempDir()
	goStub := filepath.Join(dir, "go")
	require.NoError(t, os.WriteFile(goStub, []byte(`#!/usr/bin/env bash
set -eu
if [[ $1 == build ]]; then
  cp -- "$0" "$3"
  chmod +x "$3"
elif [[ $1 == list ]]; then
  echo github.com/assurrussa/goadmin
elif [[ $1 != clean ]]; then
  printf 'ARGS:'
  printf ' <%s>' "$@"
  printf '\nAUTH:%s PRIVATE:%s WORK:%s FLAGS:%s PROXY:%s VCS:%s\n' \
    "${GOAUTH-}" "${GOPRIVATE-}" "${GOWORK-}" "${GOFLAGS-}" "${GOPROXY-}" "${GOVCS-}"
  printf 'HOME:%s CACHE:%s\n' "$HOME" "${GOMODCACHE-}"
  if [[ ${GOAUTH-} == off && ( -n ${GOAUTH_LOCAL_PATH-} || -n ${GOAUTH_VERSION-} || -n ${NETRC-} ) ]]; then exit 1; fi
fi
`), 0o600))
	require.NoError(t, os.Chmod(goStub, 0o700))
	var err error
	script, err = filepath.Abs(filepath.Join("..", "..", "scripts", "anonymous-consumer.sh"))
	require.NoError(t, err)
	env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GOAUTH_LOCAL_PATH=", "GONOTIFY_LOCAL_PATH=", "GOUPLOADS_LOCAL_PATH=", "GOWEBSOCKET_LOCAL_PATH=",
		"GOAUTH_VERSION=v999.0.0", "GOAUTH=netrc")
	return script, dir, env
}

func TestConsumerNonCandidateModesIgnoreOverrides(t *testing.T) {
	for _, mode := range []string{consumerModeSharedLocal, consumerModePublicDeps, consumerModePublished} {
		t.Run(mode, func(t *testing.T) {
			script, _, env := consumerScriptFixture(t)
			args := []string{script, mode}
			if mode == consumerModePublished {
				args = append(args, "v1.2.3")
			}
			cmd := exec.CommandContext(t.Context(), "bash", args...)
			env = append(env,
				"GOAUTH_LOCAL_PATH=/candidate-must-not-be-read", "GONOTIFY_LOCAL_PATH=/candidate-must-not-be-read",
				"GOUPLOADS_LOCAL_PATH=/candidate-must-not-be-read", "GOWEBSOCKET_LOCAL_PATH=/candidate-must-not-be-read",
				"GOAUTH=netrc", "GOPRIVATE=github.com/assurrussa/*", "GOFLAGS=-mod=vendor", "NETRC=/host-netrc")
			cmd.Env = env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			text := string(output)
			require.NotContains(t, text, "--goauth-version")
			for _, dep := range []string{"goauth", "gonotify", "gouploads", "gowebsocket"} {
				require.NotContains(t, text, "--"+dep+"-local-path")
			}
			if mode == consumerModePublished {
				require.Contains(t, text, "<--version> <v1.2.3>")
				require.NotContains(t, text, "--local-path")
			} else {
				require.Contains(t, text, "<--local-path>")
			}
			if mode != consumerModeSharedLocal {
				require.Contains(t, text, "AUTH:off PRIVATE: WORK:off FLAGS: PROXY:https://proxy.golang.org VCS:*:off")
				require.NotContains(t, text, "HOME:"+os.Getenv("HOME")+" ")
				require.Contains(t, text, "PASS: anonymous "+mode+" consumer")
			} else {
				require.Contains(t, text, "WORK:off FLAGS:")
			}
		})
	}
}

func TestConsumerCandidatesRespectExplicitPaths(t *testing.T) {
	for _, mode := range []string{consumerModeLocal, consumerModeSharedCandidates} {
		t.Run(mode, func(t *testing.T) {
			script, dir, env := consumerScriptFixture(t)
			candidate := filepath.Join(dir, "candidate with spaces")
			require.NoError(t, os.Mkdir(candidate, 0o700))
			resolved, err := filepath.EvalSymlinks(candidate)
			require.NoError(t, err)
			cmd := exec.CommandContext(t.Context(), "bash", script, mode)
			env = append(env, "GOAUTH_LOCAL_PATH="+candidate, "GONOTIFY_LOCAL_PATH="+candidate,
				"GOUPLOADS_LOCAL_PATH="+candidate, "GOWEBSOCKET_LOCAL_PATH="+candidate)
			cmd.Env = env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			for _, dep := range []string{"goauth", "gonotify", "gouploads", "gowebsocket"} {
				require.Contains(t, string(output), "<--"+dep+"-local-path> <"+resolved+">")
			}
			require.NotContains(t, string(output), "--goauth-version")
			require.Contains(t, string(output), "Local replacements are candidate evidence only.")
		})
	}
}

func TestConsumerCandidatesRequireSelectedExistingPaths(t *testing.T) {
	for _, mode := range []string{consumerModeLocal, consumerModeSharedCandidates} {
		for _, selected := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "none", true: "missing"}[selected], func(t *testing.T) {
				script, dir, env := consumerScriptFixture(t)
				cmd := exec.CommandContext(t.Context(), "bash", script, mode)
				cmd.Env = env
				if selected {
					env = append(env, "GOAUTH_LOCAL_PATH="+filepath.Join(dir, "missing candidate"))
					cmd.Env = env
				}
				output, err := cmd.CombinedOutput()
				require.Error(t, err, string(output))
				require.NotContains(t, string(output), "ARGS:")
				if !selected {
					require.Contains(t, string(output), "candidate mode requires at least one explicit dependency LOCAL_PATH")
				}
			})
		}
	}
}

func TestMakeConsumerTargetsSeparateCandidatesFromReadiness(t *testing.T) {
	script, _, env := consumerScriptFixture(t)
	repo := filepath.Dir(filepath.Dir(script))
	for _, target := range []struct{ name, mode string }{
		{"externalconsumer-local", consumerModeSharedLocal},
		{"externalconsumer-candidates", consumerModeSharedCandidates},
		{"externalconsumer-anonymous-local", consumerModeLocal},
		{"externalconsumer-public-deps-local", consumerModePublicDeps},
		{"externalconsumer-published", consumerModePublished},
	} {
		// #nosec G204 -- target names come only from the fixed test table.
		cmd := exec.CommandContext(t.Context(), "make", "-n", target.name, "VERSION=v1.2.3")
		cmd.Dir, cmd.Env = repo, env
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		require.Contains(t, string(output), "scripts/anonymous-consumer.sh "+target.mode)
	}
	for _, target := range []string{"check", "publish-readiness", "release-readiness"} {
		cmd := exec.CommandContext(t.Context(), "make", "-n", target, "VERSION=v1.2.3")
		cmd.Dir, cmd.Env = repo, env
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		text := string(output)
		require.NotContains(t, text, "anonymous-consumer.sh shared-local")
		require.NotContains(t, text, "anonymous-consumer.sh shared-candidates")
		require.NotContains(t, text, "anonymous-consumer.sh local")
		switch target {
		case "check":
			require.NotContains(t, text, "anonymous-consumer.sh")
		case "publish-readiness":
			require.Contains(t, text, "anonymous-consumer.sh public-deps-local")
		case "release-readiness":
			require.Contains(t, text, "anonymous-consumer.sh published")
		}
	}
}

func TestPublishedStarterRequireUsesModuleAtVersion(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "examples", "starter", "Dockerfile.published"))
	require.NoError(t, err)
	require.Contains(t, string(dockerfile), "go mod edit -require=github.com/assurrussa/goadmin@$GOADMIN_VERSION")
	require.NotContains(t, string(dockerfile), "-require=github.com/assurrussa/goadmin=")
}
