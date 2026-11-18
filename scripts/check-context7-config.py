#!/usr/bin/env python3
"""Offline regression checks for this repo's Context7 selection policy.

Not an implementation or validation of Context7's remote parser.
"""
import fnmatch
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
config = json.loads((ROOT / "context7.json").read_text())


def included(path):
    parts = Path(path).parts
    if parts[-1] in config["excludeFiles"]:
        return False
    ancestors = ["/".join(parts[:i]) for i in range(1, len(parts))]
    for pattern in config["excludeFolders"]:
        if "/" in pattern:
            if any(fnmatch.fnmatchcase("/".join(a.split("/")[i:]), pattern)
                   for a in ancestors for i in range(len(a.split("/")))):
                return False
        elif any(fnmatch.fnmatchcase(p, pattern) for p in parts[:-1]):
            return False
    return path == "README.md" or any(
        path.startswith(folder + "/") for folder in config["folders"]
    )


assert config["folders"], "Empty folders would index the entire repository"
assert all("/" not in p and "\\" not in p for p in config["excludeFiles"])
assert len(config["excludeFolders"]) <= 50
assert len(config["excludeFiles"]) <= 100
for key in ("folders", "excludeFolders", "excludeFiles"):
    assert len(config[key]) == len(set(config[key])), f"Duplicate {key}"
for folder in config["folders"]:
    assert (ROOT / folder).is_dir(), f"Missing included directory: {folder}"

required = [
    "README.md", "docs/consumer-guide.md", "docs/crud.md",
    "docs/first-crud.md", "docs/integration-recipes.md",
    "docs/embedding-client.md", "docs/browser-auth-sessions.md",
    "docs/assisted-recovery.md", "docs/local-mfa-policy.md",
    "docs/theme-customization.md", "docs/sso-adapter.md",
    "examples/starter/README.md", "toolkit/datagrid/README.md",
]
blocked = [
    "audit/datagrid/backend-report.md", "docs/implementation-notes.md",
    "docs/ci.md", "docs/project-context.md", "docs/security-ux-notes.md",
    "docs/design/admin-authorization-snapshot.md",
    "docs/admin-ux-verification-20261017.md", "docs/presentation-packs-qa.md",
    "docs/auth-cookie-client-followup.md", "docs/auth-session-hardening-followup.md",
    "docs/maintainers/context7-index.md", "AGENTS.md", "scripts/README.md",
]
for path in required:
    assert (ROOT / path).is_file(), f"Missing consumer page: {path}"
    assert included(path), f"Consumer page excluded: {path}"
for path in blocked:
    assert not included(path), f"Internal page included: {path}"
# Freeze the complete documented default exclusion set, not a few examples.
# These are local policy fixtures, not a claim about Context7 parser behavior.
default_folders = [
    "archive", "archived", "old", "docs/old", "deprecated", "legacy",
    "previous", "outdated", "superseded", "_snaps", "benchmark", "benchmarks",
    "benches", *[f"i18n/{lang}*" for lang in
                 ("zh", "es", "fr", "de", "ja", "ko", "ru", "pt", "it",
                  "ar", "hi", "tr", "nl", "pl", "sv", "vi", "th")],
    "zh-cn", "zh-tw", "zh-hk", "zh-mo", "zh-sg",
]
default_files = [
    "CHANGELOG.md", "changelog.md", "CHANGELOG.mdx", "changelog.mdx",
    "LICENSE.md", "license.md", "CODE_OF_CONDUCT.md", "code_of_conduct.md",
    "NEWS.md",
]
for pattern in default_folders:
    assert pattern in config["excludeFolders"], f"Default folder missing: {pattern}"
    assert not included("docs/" + pattern.replace("*", "-test") + "/guide.md")
for filename in default_files:
    assert filename in config["excludeFiles"], f"Default filename missing: {filename}"
    assert not included("docs/" + filename)
assert not included("examples/starter/AGENTS.md")
for path in ["docs/tutorials/new-consumer-guide.md", "docs/notifications.md",
             "docs/inertia.md", "docs/themes.md", "docs/module-selection.md"]:
    assert included(path), f"New consumer guide excluded: {path}"
assert included("toolkit/formvalidator/README.md")

paths = subprocess.check_output(
    ["git", "ls-files", "-z"], cwd=ROOT
).decode().split("\0")
pages = sorted(p for p in paths if p.endswith(".md") and included(p))
print(f"PASS: selection policy; {len(pages)} tracked Markdown pages selected")
print("\n".join(pages))
