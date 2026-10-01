#!/usr/bin/env bash
# Snapshot candidate sources, including working changes but excluding ignored caches.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
work=$(mktemp -d "${TMPDIR:-/tmp}/goadmin-starter.XXXXXXXX")
trap 'rm -rf -- "$work"' EXIT
for dep in goauth gonotify gouploads gowebsocket; do
  source=$(cd -- "$repo/../$dep" && pwd -P)
  mkdir -p "$work/$dep"
  git -C "$source" ls-files --cached --others --exclude-standard -z > "$work/files"
  # Deleted tracked files are omitted, so a dirty candidate is represented faithfully.
  python3 - "$source" "$work/$dep" "$work/files" <<'PY'
import os
import pathlib
import shutil
import sys
source, target, manifest = map(pathlib.Path, sys.argv[1:])
for name in manifest.read_bytes().split(b'\0'):
    if not name:
        continue
    relative = pathlib.Path(os.fsdecode(name))
    entry = source / relative
    if not entry.is_file() and not entry.is_symlink():
        continue
    destination = target / relative
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(entry, destination, follow_symlinks=False)
PY
done
export GOADMIN_CANDIDATE_CONTEXTS="$work"
docker compose --project-directory "$repo/examples/starter" \
  -f "$repo/examples/starter/compose.yaml" -f "$repo/examples/starter/compose.local.yaml" "$@"
