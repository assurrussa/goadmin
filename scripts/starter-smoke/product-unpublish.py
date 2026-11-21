"""Run the Products write pilot with published GoAdmin, Firefox and a real stdio process."""
import base64
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    harness = Path(__file__).resolve().parent
    starter = harness.parents[1] / "examples/starter"
    with tempfile.TemporaryDirectory(prefix="product-unpublish-") as work:
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        env = dict(os.environ, GOWORK="off", POSTGRES_USER="fixture", POSTGRES_DB="fixture",
                   POSTGRES_PASSWORD=secrets.token_hex(24), ADMIN_MODULES="access,users,products,product-unpublish-mcp",
                   ADMIN_URL=f"http://localhost:{port}", STARTER_ADMIN_PORT=str(port),
                   MCP_PAIRING_TRUSTED_PROXY_IP="", APP_ADMIN_CANONICAL_SESSION_BACKEND="pgsql",
                   CSRF_SECRET=secrets.token_hex(32), FIRST_ADMIN_SETUP_TOKEN=secrets.token_hex(32),
                   UNPUBLISH_QA_PROJECT="goadmin-unpublish-"+secrets.token_hex(6),
                   UNPUBLISH_QA_CLI=str(Path(work) / "product-unpublish-mcp"))
        for key in ("GOAUTH_SIGNING_KEY", "GOAUTH_TOKEN_KEY", "GOAUTH_ENVELOPE_KEY"):
            env[key] = base64.b64encode(secrets.token_bytes(32)).decode()
        env["GOADMIN_VERSION"] = subprocess.check_output(
            ["go", "list", "-m", "-f", "{{.Version}}", "github.com/assurrussa/goadmin"],
            text=True, cwd=starter, env=env).strip()
        subprocess.run(["go", "build", "-mod=readonly", "-o", env["UNPUBLISH_QA_CLI"], "./cmd/product-unpublish-mcp"],
                       cwd=starter, env=env, check=True)
        compose = ["docker", "compose", "-p", env["UNPUBLISH_QA_PROJECT"], "-f", str(starter / "compose.yaml")]
        try:
            subprocess.run([*compose, "up", "--build", "-d", "admin"], env=env, check=True)
            for attempt in range(90):
                try:
                    with urllib.request.urlopen(env["ADMIN_URL"]+"/auth/login", timeout=2) as response:
                        if response.status == 200:
                            break
                except (ConnectionError, TimeoutError, urllib.error.URLError):
                    pass
                if attempt % 5 == 4:
                    running = subprocess.check_output([*compose, "ps", "--status", "running", "--services"], text=True, env=env)
                    if "admin" not in running.splitlines():
                        raise RuntimeError("isolated starter exited during setup")
                time.sleep(1)
            else:
                raise RuntimeError("isolated starter did not become ready")
            subprocess.run(["node", str(harness / "product-unpublish.mjs")], cwd=starter, env=env, check=True)
        finally:
            subprocess.run([*compose, "down", "-v", "--rmi", "local"], env=env, check=True)


if __name__ == "__main__":
    main()
