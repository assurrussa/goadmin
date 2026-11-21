"""Exercise the starter through a trusted local TLS proxy with the standard stdio client."""
import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def run(*cmd, cwd=None, env=None, capture=False):
    return subprocess.run(cmd, cwd=cwd, env=env, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def certificates(work):
    ca, leaf = work / "ca.pem", work / "leaf.pem"
    run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2", "-subj",
        "/CN=Starter QA CA", "-keyout", work / "ca.key", "-out", ca,
        "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign",
        "-addext", "subjectKeyIdentifier=hash")
    run("openssl", "req", "-newkey", "rsa:2048", "-nodes", "-subj", "/CN=localhost",
        "-keyout", work / "leaf.key", "-out", work / "leaf.csr")
    (work / "san").write_text("subjectAltName=DNS:localhost\nextendedKeyUsage=serverAuth\n"
                              "authorityKeyIdentifier=keyid:always\nsubjectKeyIdentifier=hash\n")
    run("openssl", "x509", "-req", "-in", work / "leaf.csr", "-CA", ca, "-CAkey", work / "ca.key",
        "-CAcreateserial", "-days", "2", "-out", leaf, "-extfile", work / "san")
    profile = work / "firefox"
    profile.mkdir()
    run("certutil", "-N", "-d", "sql:" + str(profile), "--empty-password")
    run("certutil", "-A", "-d", "sql:" + str(profile), "-n", "Starter QA",
        "-t", "C,,", "-i", ca)


def build(work, root, published):
    arch = run("docker", "info", "--format", "{{.Architecture}}", capture=True).strip()
    env = dict(os.environ, GOOS="linux", GOARCH={"x86_64": "amd64", "aarch64": "arm64"}.get(arch, arch),
               GOWORK="off", CGO_ENABLED="0")
    starter = root / "examples/starter"
    mode = "-mod=readonly"
    if published:
        dest = work / "source"
        shutil.copytree(starter, dest, ignore=shutil.ignore_patterns("node_modules", ".git", "tmp"))
        starter = dest
        for module in ("goadmin", "goauth", "gonotify", "gouploads", "gowebsocket"):
            run("go", "mod", "edit", "-dropreplace=github.com/assurrussa/" + module, cwd=starter, env=env)
        run("go", "mod", "edit", "-require=github.com/assurrussa/goadmin@" + published, cwd=starter, env=env)
        env.update(GOAUTH="off", GOPRIVATE="", GONOPROXY="", GONOSUMDB="", GOPROXY="https://proxy.golang.org",
                   GOSUMDB="sum.golang.org", GOVCS="*:off")
        mode = "-mod=mod"
    for name, pkg in (("admin", "."), ("goadmin-mcp", "github.com/assurrussa/goadmin/cmd/goadmin-mcp")):
        run("go", "build", mode, "-o", work / name, pkg, cwd=starter, env=env)
    run("go", "build", "-o", work / "edge", root / "scripts/starter-smoke/testdata/edge.go", env=env)


def exercise(work, root, args):
    certificates(work)
    build(work, root, args.published)
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    origin = f"https://localhost:{port}"
    project = "goadmin-unpublish-" + secrets.token_hex(6)
    composefile = work / "compose.json"
    compose = ["docker", "compose", "-p", project, "-f", str(composefile)]
    database = dict(POSTGRES_USER="fixture", POSTGRES_DB="fixture", POSTGRES_PASSWORD=secrets.token_hex(24))
    setup = secrets.token_hex(32)
    admin = dict(database, ADMIN_URL=origin, FIRST_ADMIN_SETUP_TOKEN=setup, CSRF_SECRET=secrets.token_hex(32),
                 ADMIN_MODULES="access,users,products,product-unpublish-mcp", STARTER_MIGRATE="1",
                 APP_ADMIN_CANONICAL_SESSION_BACKEND=args.sessions, REDIS_DSN="redis://redis:6379/0",
                 MCP_PAIRING_TRUSTED_PROXY_IP="${QA_EDGE_IP:-}", UPLOAD_ROOT="/tmp/uploads")
    for key in ("GOAUTH_SIGNING_KEY", "GOAUTH_TOKEN_KEY", "GOAUTH_ENVELOPE_KEY"):
        admin[key] = base64.b64encode(secrets.token_bytes(32)).decode()
    def mount(name):
        return f"{work / name}:/qa/{name}:ro"
    image = "postgres:17-alpine"
    services = {
        "postgres": {"image": image, "environment": database, "healthcheck": {
            "test": ["CMD", "pg_isready", "-U", "fixture", "-d", "fixture"], "interval": "2s", "retries": 30}},
        "edge": {"image": image, "entrypoint": ["/qa/edge", "-listen", f":{port}"],
                 "ports": [f"127.0.0.1:{port}:{port}"],
                 "volumes": [mount(n) for n in ("edge", "leaf.pem", "leaf.key", "ca.pem", "goadmin-mcp")]},
        "admin": {"image": image, "entrypoint": ["/qa/admin"], "volumes": [mount("admin")], "environment": admin,
                  "depends_on": {"postgres": {"condition": "service_healthy"}}}}
    if args.sessions == "redis":
        services["redis"] = {"image": "redis:7-alpine"}
        services["admin"]["depends_on"]["redis"] = {"condition": "service_started"}
    composefile.write_text(json.dumps({"services": services}))
    try:
        run(*compose, "up", "-d", "postgres", "edge")
        edge = run(*compose, "ps", "-q", "edge", capture=True).strip()
        ip = run("docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
                 edge, capture=True).strip()
        env = dict(os.environ, QA_EDGE_IP=ip, ADMIN_URL=origin, FIRST_ADMIN_SETUP_TOKEN=setup,
                   UNPUBLISH_QA_PROFILE=str(work / "firefox"), UNPUBLISH_QA_COMPOSE=str(composefile),
                   NODE_EXTRA_CA_CERTS=str(work / "ca.pem"), UNPUBLISH_QA_PROJECT=project,
                   UNPUBLISH_QA_CLI=str(work / "client"))
        (work / "client").write_text("#!/bin/sh\nexec docker exec -i -e SSL_CERT_FILE=/qa/ca.pem " +
                                     edge + ' /qa/goadmin-mcp -resource-path /agent/product-unpublish "$@"\n')
        (work / "client").chmod(0o700)
        run(*compose, "up", "-d", "admin", env=env)
        tls = ssl.create_default_context(cafile=str(work / "ca.pem"))
        for _ in range(90):
            try:
                with urllib.request.urlopen(origin + "/healthz", timeout=3, context=tls) as response:
                    if response.status == 204:
                        break
            except (OSError, urllib.error.URLError):
                time.sleep(1)
        else:
            raise RuntimeError("starter did not pass readiness over trusted HTTPS")
        run("node", root / "scripts/starter-smoke/product-unpublish.mjs", cwd=root / "examples/starter", env=env)
    finally:
        run(*compose, "down", "-v")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--candidate", action="store_true")
    source.add_argument("--published", metavar="TAG")
    parser.add_argument("--sessions", choices=("pgsql", "redis"), default="pgsql")
    with tempfile.TemporaryDirectory(prefix="confirmed-https-") as workspace:
        exercise(Path(workspace), Path(__file__).resolve().parents[2], parser.parse_args())
