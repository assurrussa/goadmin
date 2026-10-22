#!/usr/bin/env python3
"""Isolated, opt-in real-host acceptance. Never reads the starter's .env."""
import argparse
import base64
import datetime
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request

REPO = Path(__file__).resolve().parent.parent
ARTIFACTS = REPO / 'output/playwright/starter-smoke'
RUN_ID = re.compile(r'^\d{8}t\d{6}z-[a-f0-9]{8}$')
MODULES = 'access,jobs,uploads,queues,realtime'
MARKER = 'goadmin-starter-smoke-v1'


def protected_write(path, contents):
    with open(path, 'x', opener=lambda p, flags: os.open(p, flags, 0o600)) as file:
        file.write(contents)


def command(args, *, output=None, check=True, cwd=REPO):
    # Shell/project env cannot accidentally override the generated Compose config.
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('COMPOSE_', 'POSTGRES_', 'GOAUTH_', 'ADMIN_', 'STARTER_'))
           and key not in ('CSRF_SECRET', 'FIRST_ADMIN_SETUP_TOKEN')}
    if output is not None:
        with output.open('w') as log:
            result = subprocess.run(args, cwd=cwd, env=env, stdout=log, stderr=subprocess.STDOUT)
    else:
        result = subprocess.run(args, cwd=cwd, env=env)
    if check and result.returncode:
        raise RuntimeError(f'{args[0]} failed ({result.returncode}); see {output or "terminal"}')
    return result.returncode


def compose(case, *args, output=None, check=True):
    return command(['docker', 'compose', '--project-directory', str(Path(case['env']).parent),
                    '--env-file', case['env'], '-p', case['project'], '-f', case['compose'], *args],
                   output=output, check=check)


def manifest_path(run_id):
    if not RUN_ID.fullmatch(run_id):
        raise ValueError('RUN_ID must be an ID printed by starter-smoke, e.g. 20261022t120000z-1234abcd')
    folder = ARTIFACTS / run_id
    if folder.is_symlink():
        raise ValueError('refusing symlink artifact directory')
    return folder / 'run.json'


def read_manifest(run_id):
    path = manifest_path(run_id)
    data = json.loads(path.read_text())
    if data.get('owner') != MARKER or data.get('runId') != run_id:
        raise ValueError('not an owned starter smoke run')
    for name, case in data['cases'].items():
        expected = f'goadmin-smoke-{run_id}-{name}'
        if name not in ('default', 'topbar') or case['project'] != expected:
            raise ValueError('unexpected Compose project')
        if Path(case['env']) != path.parent / f'{name}.env' or Path(case['compose']) != path.parent / f'{name}.compose.json':
            raise ValueError('unexpected Compose paths')
        for file in (Path(case['env']), Path(case['compose'])):
            if file.is_symlink():
                raise ValueError('refusing symlink run configuration')
        config = json.loads(Path(case['compose']).read_text())
        if config.get('name') != expected or config.get('volumes') != {'postgres_data': {}, 'uploads': {}}:
            raise ValueError('refusing changed project or storage ownership')
    return data


def snapshot(source, destination):
    """Same source boundary as starter-local.sh: tracked and nonignored files."""
    names = subprocess.check_output(['git', '-C', str(source), 'ls-files', '--cached', '--others',
                                     '--exclude-standard', '-z']).split(b'\0')
    destination.mkdir(parents=True)
    for name in names:
        if not name:
            continue
        relative = Path(os.fsdecode(name))
        entry = source / relative
        if not entry.is_file() and not entry.is_symlink():
            continue
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(entry, target, follow_symlinks=False)


def spec(project, port, manifest, candidates):
    return {'name': project, 'services': {
        'postgres': {'image': 'postgres:17-alpine',
                     'environment': {key: '${' + key + ':?required}' for key in
                                     ('POSTGRES_USER', 'POSTGRES_PASSWORD', 'POSTGRES_DB')},
                     'volumes': ['postgres_data:/var/lib/postgresql/data'],
                     'healthcheck': {'test': ['CMD-SHELL', 'pg_isready -U $$POSTGRES_USER -d $$POSTGRES_DB'],
                                     'interval': '2s', 'timeout': '5s', 'retries': 40}},
        'admin': {'image': f'{project}:source',
                  'build': {'context': str(REPO), 'dockerfile': 'examples/starter/Dockerfile',
                            'args': {'VITE_ADMIN_EXTENSION_MANIFESTS': manifest},
                            'additional_contexts': {dep: str(candidates / dep) for dep in
                                                    ('goauth', 'gonotify', 'gouploads', 'gowebsocket')}},
                  'depends_on': {'postgres': {'condition': 'service_healthy'}},
                  'environment': {**{key: '${' + key + ':?required}' for key in (
                      'POSTGRES_USER', 'POSTGRES_PASSWORD', 'POSTGRES_DB', 'ADMIN_URL', 'CSRF_SECRET',
                      'FIRST_ADMIN_SETUP_TOKEN', 'GOAUTH_SIGNING_KEY', 'GOAUTH_TOKEN_KEY', 'GOAUTH_ENVELOPE_KEY')},
                      'ADMIN_MODULES': MODULES, 'APP_ADMIN_CANONICAL_SESSION_BACKEND': 'pgsql',
                      'STARTER_MIGRATE': '1', 'UPLOAD_ROOT': '/data/uploads'},
                  'ports': [f'127.0.0.1:{port}:8080'], 'volumes': ['uploads:/data/uploads']}},
        'volumes': {'postgres_data': {}, 'uploads': {}}}


def wait_ready(url, timeout=120):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url + '/healthz', timeout=3) as response:
                if response.status in (200, 204):
                    return
        except (OSError, urllib.error.URLError) as error:
            last = error
        time.sleep(0.5)  # Readiness polling, never browser timing.
    raise RuntimeError(f'{url} did not become ready: {last}')


def diagnostics(data):
    for name, case in data['cases'].items():
        folder = Path(case['env']).parent
        compose(case, 'logs', '--no-color', output=folder / f'{name}.containers.log', check=False)
        compose(case, 'ps', '--all', '--format', 'json', output=folder / f'{name}.containers.json', check=False)
        compose(case, 'exec', '-T', 'admin', 'cat', '/usr/local/share/goadmin-starter/modules.json',
                output=folder / f'{name}.modules.json', check=False)
        # Inspect images only: container inspection would expose environment secrets.
        command(['docker', 'image', 'inspect', f'{case["project"]}:source', 'postgres:17-alpine'],
                output=folder / f'{name}.images.json', check=False)


def resolved_modules(contents):
    """go list emits adjacent JSON objects, not a JSON array."""
    decoder = json.JSONDecoder()
    modules = []
    remaining = contents.lstrip()
    while remaining:
        module, end = decoder.raw_decode(remaining)
        if not isinstance(module, dict):
            raise ValueError('expected go list module object')
        item = {'path': module['Path'], 'version': module.get('Version'), 'main': module.get('Main', False)}
        if module.get('Replace'):
            replacement = module['Replace']
            item['replacement'] = {'path': replacement['Path'], 'version': replacement.get('Version')}
        modules.append(item)
        remaining = remaining[end:].lstrip()
    return modules


def image_identities(contents):
    # Never copy image Config/Env, nor inspect containers to create this summary.
    return [{'id': image['Id'], 'tags': image.get('RepoTags', []),
             'digests': image.get('RepoDigests', []), 'os': image.get('Os'),
             'architecture': image.get('Architecture')}
            for image in json.loads(contents)]


def redact_report(report, folder, case_names):
    variants = set()
    for name in case_names:
        try:
            credentials = json.loads((folder / f'{name}.credentials.json').read_text())
        except (OSError, ValueError):
            continue
        if not isinstance(credentials, dict):
            continue
        for key in ('password', 'setupToken'):
            secret = credentials.get(key)
            if not isinstance(secret, str) or not secret:
                continue
            # Playwright call logs can contain JSON-quoted or repr-quoted fill values.
            encoded = {json.dumps(secret, ensure_ascii=ascii_only)[1:-1] for ascii_only in (True, False)}
            encoded.add(repr(secret)[1:-1])
            variants.update((secret, *encoded))
            variants.update(json.dumps(value)[1:-1] for value in encoded)
    replacements = sorted(variants, key=len, reverse=True)

    def redact(value):
        if isinstance(value, str):
            for secret in replacements:
                value = value.replace(secret, '[REDACTED]')
            return value
        if isinstance(value, dict):
            return {redact(key): redact(item) for key, item in value.items()}
        if isinstance(value, list):
            return [redact(item) for item in value]
        return value

    return redact(report)


def write_report(data, folder, collection_errors=None):
    errors = list(collection_errors or [])

    def load(name, parse, fallback):
        try:
            return parse((folder / name).read_text())
        except (OSError, ValueError, KeyError, TypeError) as error:
            errors.append({'artifact': name, 'error': type(error).__name__})
            return fallback

    def browser_report(contents):
        parsed = json.loads(contents)
        if not isinstance(parsed, dict) or not isinstance(parsed.get('cases', {}), dict):
            raise ValueError('expected browser report object')
        return parsed

    browser = load('browser-report.json', browser_report, {})
    if browser and browser.get('runId') != data['runId']:
        errors.append({'artifact': 'browser-report.json', 'error': 'RunIdMismatch'})
        browser = {}
    report = {key: data[key] for key in ('runId', 'revision', 'dirty', 'modules', 'result')}
    report.update({'schemaVersion': 1, 'cases': {}, 'diagnosticErrors': errors,
                   'browser': {key: browser[key] for key in (
                       'browser', 'playwright', 'startedAt', 'finishedAt', 'diagnosticOnly') if key in browser},
                   'artifacts': {'browserReport': 'browser-report.json', 'browserLog': 'browser.log'}})
    if 'error' in data:
        report['error'] = data['error']
    for name, case in data['cases'].items():
        report['cases'][name] = {
            'project': case['project'], 'url': case['url'],
            'dependencies': load(f'{name}.modules.json', resolved_modules, []),
            'images': load(f'{name}.images.json', image_identities, []),
            'scenarios': browser.get('cases', {}).get(name),
            'artifacts': {'buildLog': f'{name}.build.log', 'modules': f'{name}.modules.json',
                          'images': f'{name}.images.json', 'containerLogs': f'{name}.containers.log',
                          'containerSummary': f'{name}.containers.json', 'trace': f'{name}-trace.zip',
                          'screenshots': sorted(path.name for path in folder.glob(f'{name}-*.png'))},
        }
    report = redact_report(report, folder, data['cases'])
    contents = json.dumps(report, indent=2) + '\n'
    target = folder / 'report.json'
    if target.exists():
        target.write_text(contents)
    else:
        protected_write(target, contents)
    return report


def down(data):
    failed = False
    for name, case in data['cases'].items():
        print(f'Removing owned project {case["project"]}', flush=True)
        failed |= compose(case, 'down', '--volumes', '--remove-orphans',
                          output=Path(case['env']).parent / f'{name}.down.log', check=False) != 0
    if failed:
        raise RuntimeError('cleanup failed; inspect *.down.log and rerun starter-smoke-down')


def run():
    os.umask(0o077)
    run_id = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dt%H%M%Sz-') + secrets.token_hex(4)
    folder = manifest_path(run_id).parent
    folder.mkdir(parents=True, mode=0o700)
    data = {'owner': MARKER, 'runId': run_id, 'revision': subprocess.check_output(
        ['git', 'rev-parse', 'HEAD'], cwd=REPO, text=True).strip(),
        'dirty': bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=REPO)),
        'modules': MODULES.split(','), 'cases': {}, 'result': 'running'}
    candidates = folder / 'candidates'
    for dep in ('goauth', 'gonotify', 'gouploads', 'gowebsocket'):
        snapshot(REPO.parent / dep, candidates / dep)
    for name, host, port in [('default', 'localhost', os.environ.get('SMOKE_DEFAULT_PORT', '18768')),
                             ('topbar', '127.0.0.1', os.environ.get('SMOKE_TOPBAR_PORT', '18769'))]:
        if not port.isdigit() or not 1024 <= int(port) <= 65535:
            raise ValueError('smoke ports must be integers between 1024 and 65535')
        project = f'goadmin-smoke-{run_id}-{name}'
        url = f'http://{host}:{port}'
        setup = secrets.token_urlsafe(40)
        password = 'Smoke-' + secrets.token_urlsafe(24) + '-9a!'
        env = {'POSTGRES_USER': 'smoke', 'POSTGRES_PASSWORD': secrets.token_hex(24), 'POSTGRES_DB': 'smoke',
               'ADMIN_URL': url, 'CSRF_SECRET': secrets.token_hex(32), 'FIRST_ADMIN_SETUP_TOKEN': setup,
               **{key: base64.b64encode(secrets.token_bytes(32)).decode() for key in
                  ('GOAUTH_SIGNING_KEY', 'GOAUTH_TOKEN_KEY', 'GOAUTH_ENVELOPE_KEY')}}
        env_path = folder / f'{name}.env'
        protected_write(env_path, ''.join(f'{key}={value}\n' for key, value in env.items()))
        compose_path = folder / f'{name}.compose.json'
        protected_write(compose_path, json.dumps(spec(project, port,
            'examples/theme-topbar/extension.json' if name == 'topbar' else '', candidates), indent=2))
        data['cases'][name] = {'project': project, 'env': str(env_path), 'compose': str(compose_path), 'url': url}
        protected_write(folder / f'{name}.credentials.json', json.dumps(
            {'url': url, 'setupToken': setup, 'email': f'smoke-{name}@example.test', 'password': password,
             'restrictedEmail': f'queues-{name}@example.test'}, indent=2))
    protected_write(folder / 'run.json', json.dumps(data, indent=2) + '\n')
    print(f'Starter smoke RUN_ID={run_id}\nArtifacts: {folder}', flush=True)
    keep = os.environ.get('KEEP') == '1'
    try:
        for name, case in data['cases'].items():
            print(f'Building and starting {name}: {case["url"]}', flush=True)
            compose(case, 'up', '--build', '-d', output=folder / f'{name}.build.log')
            wait_ready(case['url'])
        harness = REPO / 'scripts/starter-smoke'
        command(['npm', 'ci', '--ignore-scripts', '--no-audit', '--no-fund'], cwd=harness,
                output=folder / 'browser-install.log')
        command(['npm', 'exec', '--', 'playwright', 'install', 'chromium'], cwd=harness,
                output=folder / 'chromium-install.log')
        command(['node', 'run.mjs', str(folder)], cwd=harness, output=folder / 'browser.log')
        data['result'] = 'passed'
    except BaseException as error:
        data['result'] = 'failed'
        data['error'] = str(error)
        raise
    finally:
        (folder / 'run.json').write_text(json.dumps(data, indent=2) + '\n')
        collection_errors = []
        try:
            diagnostics(data)
        except Exception as error:
            collection_errors.append({'stage': 'collection', 'error': type(error).__name__})
        try:
            write_report(data, folder, collection_errors)
        except Exception as error:
            # A failed optional diagnostic must not replace the build/browser failure.
            print(f'WARNING: report.json unavailable ({type(error).__name__})', file=sys.stderr)
        if keep:
            print(f'KEEP=1: stacks retained. Private credentials: {folder}/*.credentials.json\n'
                  f'Cleanup: make starter-smoke-down RUN_ID={run_id}', flush=True)
        else:
            down(data)
        print(f'Starter smoke: {data["result"]}; artifacts: {folder}', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('up', 'down'))
    parser.add_argument('--run-id', default='')
    args = parser.parse_args()
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    try:
        if args.action == 'down':
            down(read_manifest(args.run_id))
        else:
            run()
    except (RuntimeError, ValueError, OSError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
