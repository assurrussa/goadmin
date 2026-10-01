#!/usr/bin/env bash
# Build the stdlib-only probe, then isolate all consumer resolution from this host.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
mode=${1:-}
case "$mode" in
  local|shared-candidates|shared-local|public-deps-local) [[ $# -eq 1 ]] || exit 2 ;;
  published) [[ $# -eq 2 && $2 =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || exit 2 ;;
  *) echo 'usage: anonymous-consumer.sh local | shared-candidates | shared-local | public-deps-local | published <exact-tag>' >&2; exit 2 ;;
esac
go_bin=$(command -v go)
work=$(mktemp -d "${TMPDIR:-/tmp}/goadmin-consumer.XXXXXXXX")
mkdir -p "$work/home" "$work/config" "$work/tmp" "$work/bin"
anonymous() {
  env -i PATH="$work/bin:$PATH" HOME="$work/home" XDG_CONFIG_HOME="$work/config" \
    TMPDIR="$work/tmp" GOPATH="$work/gopath" GOMODCACHE="$work/modules" GOCACHE="$work/build" \
    GOENV=off GOWORK=off GOFLAGS= GOTOOLCHAIN=local GOAUTH=off GOPRIVATE= GONOPROXY= GONOSUMDB= \
    GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org 'GOVCS=*:off' \
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null \
    GIT_CONFIG_COUNT=0 GIT_TERMINAL_PROMPT=0 "$@"
}
cleanup() {
  result=$?
  trap - EXIT
  if [[ -d "$work/modules" ]]; then anonymous "$go_bin" clean -modcache || result=1; fi
  rm -rf -- "$work"
  exit "$result"
}
trap cleanup EXIT
ln -s "$go_bin" "$work/bin/go"
(cd "$repo" && GOWORK=off "$go_bin" build -o "$work/probe" ./cmd/externalconsumerprobe)
args=(--timeout 10m)
if [[ $mode != published ]]; then
  args+=(--local-path "$repo")
  if [[ $mode == local || $mode == shared-candidates ]]; then
    selected=0
    for dep in goauth gonotify gouploads gowebsocket; do
      case "$dep" in
        goauth) path=${GOAUTH_LOCAL_PATH-} ;;
        gonotify) path=${GONOTIFY_LOCAL_PATH-} ;;
        gouploads) path=${GOUPLOADS_LOCAL_PATH-} ;;
        gowebsocket) path=${GOWEBSOCKET_LOCAL_PATH-} ;;
      esac
      if [[ -n $path ]]; then
        selected=$((selected + 1))
        path=$(cd -- "$repo" && cd -- "$path" && pwd -P)
        args+=("--$dep-local-path" "$path")
      fi
    done
    if [[ $selected -eq 0 ]]; then
      echo 'candidate mode requires at least one explicit dependency LOCAL_PATH' >&2
      exit 2
    fi
  fi
else
  args+=(--version "$2")
fi
cd "$work"
if [[ $mode == shared-local || $mode == shared-candidates ]]; then
  GOWORK=off GOFLAGS= "$work/probe" "${args[@]}"
  if [[ $mode == shared-candidates ]]; then
    printf 'Local replacements are candidate evidence only.\n'
  fi
  exit
fi
anonymous "$work/probe" "${args[@]}"
printf 'PASS: anonymous %s consumer\n' "$mode"
if [[ $mode == local ]]; then
  printf 'Local replacements are candidate evidence only.\n'
fi
