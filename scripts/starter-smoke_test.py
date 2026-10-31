import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
loader = importlib.util.spec_from_file_location('smoke', Path(__file__).with_name('starter-smoke.py'))
smoke = importlib.util.module_from_spec(loader)
loader.loader.exec_module(smoke)


class IsolationTests(unittest.TestCase):
    def test_report_redacts_fill_secrets_and_escaped_forms_without_losing_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            password = 'unique-password"with\\slash\nnewline\t\u03a9'
            token = "unique-setup'token\\with\rreturn\u0416"
            (folder / 'default.credentials.json').write_text(json.dumps({
                'password': password, 'setupToken': token, 'email': 'fixture@example.test'}))
            variants = [password, token, json.dumps(password)[1:-1], json.dumps(token)[1:-1],
                        json.dumps(password, ensure_ascii=False)[1:-1], repr(token)[1:-1],
                        json.dumps(json.dumps(password)[1:-1])[1:-1]]
            stack = '\n'.join(f'locator.fill("{value}") timed out' for value in variants)
            paint = {'light': {'edit': {'background': {'css': 'rgb(15, 118, 108)', 'rgba': [15, 118, 108, 255]}}}}
            checks = ['real WebSocket completion', 'paint matched the selected theme']
            scenarios = {'result': 'failed', 'error': {'stack': stack, 'message': token},
                         'consoleErrors': [password, {'message': json.dumps(token)[1:-1]}],
                         'checks': checks, 'paint': paint}
            (folder / 'browser-report.json').write_text(json.dumps({
                'runId': 'test-run', 'cases': {'default': scenarios}}))
            data = {'runId': 'test-run', 'revision': 'abc123', 'dirty': True, 'modules': ['access'],
                    'result': 'failed', 'error': f'fill failed: {password}',
                    'cases': {'default': {'project': 'owned-project', 'url': 'http://localhost:18768'}}}
            report = smoke.write_report(data, folder)
            saved = (folder / 'report.json').read_text()
            decoded = json.loads(saved)
            for secret in variants:
                self.assertNotIn(secret, saved)
                self.assertNotIn(secret, json.dumps(decoded, ensure_ascii=False))
            result = report['cases']['default']['scenarios']
            self.assertEqual(result['checks'], checks)
            self.assertEqual(result['paint'], paint)
            self.assertEqual(result['error']['message'], '[REDACTED]')
            self.assertEqual(result['error']['stack'].count('[REDACTED]'), len(variants))
            self.assertEqual(report['error'], 'fill failed: [REDACTED]')
            self.assertEqual(data['error'], f'fill failed: {password}')
            self.assertNotIn('credentials', saved)
            self.assertNotIn('fixture@example.test', saved)

    def test_aggregate_resolves_module_stream_and_selects_only_image_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            data = {'runId': 'test-run', 'revision': 'abc123', 'dirty': True,
                    'modules': ['access', 'jobs'], 'result': 'passed',
                    'cases': {'default': {'project': 'owned-project', 'url': 'http://localhost:18768'}}}
            module = {'Path': 'example.com/starter', 'Main': True, 'Dir': '/machine/private/starter'}
            dependency = {'Path': 'github.com/assurrussa/goadmin', 'Version': 'v0.7.0',
                          'Replace': {'Path': '../..', 'Dir': '/machine/private/goadmin'}}
            published = {'Path': 'github.com/assurrussa/goauth', 'Version': 'v0.12.0'}
            (folder / 'default.modules.json').write_text(
                '\n' + json.dumps(module) + '\n\n' + json.dumps(dependency) + json.dumps(published) + '\n')
            (folder / 'default.images.json').write_text(json.dumps([{
                'Id': 'sha256:actual-image', 'RepoTags': ['owned-project:source'],
                'RepoDigests': ['registry/image@sha256:published'], 'Os': 'linux', 'Architecture': 'arm64',
                'Config': {'Env': ['SIGNING_KEY=never-copy-this-secret']},
                'ContainerConfig': {'Env': ['PASSWORD=never-copy-this-secret']}}]))
            scenarios = {'result': 'passed', 'checks': ['real WebSocket completion'], 'paint': {'light': {}}}
            (folder / 'browser-report.json').write_text(json.dumps({
                'runId': data['runId'], 'profile': 'record-editing', 'browser': '144.0', 'playwright': 'pinned',
                'cases': {'default': scenarios}}))
            (folder / 'default.credentials.json').write_text('never-copy-this-secret')
            report = smoke.write_report(data, folder)
            self.assertEqual(report['result'], 'passed')
            self.assertEqual(report['profile'], 'record-editing')
            self.assertEqual(report['revision'], 'abc123')
            case = report['cases']['default']
            self.assertEqual(case['dependencies'], [
                {'path': 'example.com/starter', 'version': None, 'main': True},
                {'path': 'github.com/assurrussa/goadmin', 'version': 'v0.7.0', 'main': False,
                 'replacement': {'path': '../..', 'version': None}},
                {'path': 'github.com/assurrussa/goauth', 'version': 'v0.12.0', 'main': False}])
            self.assertEqual(case['images'], [{'id': 'sha256:actual-image', 'tags': ['owned-project:source'],
                'digests': ['registry/image@sha256:published'], 'os': 'linux', 'architecture': 'arm64'}])
            self.assertEqual(case['scenarios'], scenarios)
            self.assertEqual(case['artifacts']['modules'], 'default.modules.json')
            self.assertEqual(report['diagnosticErrors'], [])
            saved = (folder / 'report.json').read_text()
            self.assertNotIn('never-copy-this-secret', saved)
            self.assertNotIn('/machine/private', saved)
            self.assertNotIn('credentials', saved)
            self.assertEqual((folder / 'report.json').stat().st_mode & 0o777, 0o600)

    def test_missing_or_invalid_diagnostics_preserve_failed_build_result(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            data = {'runId': 'test-run', 'revision': 'abc123', 'dirty': True,
                    'modules': ['access'], 'profile': 'record-editing', 'result': 'failed', 'error': 'build failed',
                    'cases': {'default': {'project': 'owned-project', 'url': 'http://localhost:18768'}}}
            (folder / 'default.modules.json').write_text('service admin is not running')
            (folder / 'browser-report.json').write_text('[]')
            report = smoke.write_report(data, folder, [{'stage': 'collection', 'error': 'OSError'}])
            self.assertEqual(report['result'], 'failed')
            self.assertEqual(report['error'], 'build failed')
            self.assertEqual(report['profile'], 'record-editing')
            self.assertEqual(data['error'], 'build failed')
            self.assertEqual(report['cases']['default']['dependencies'], [])
            self.assertEqual(report['cases']['default']['images'], [])
            self.assertIsNone(report['cases']['default']['scenarios'])
            self.assertEqual(len(report['diagnosticErrors']), 4)

    def test_assets_stage_copies_server_sources_read_by_resource_tests(self):
        repo = Path(__file__).resolve().parent.parent
        asset_stage = (repo / 'examples/starter/Dockerfile').read_text().split(
            'FROM golang:', 1)[0]
        copies = re.findall(r'^COPY ([^ ]+) ([^ ]+)$', asset_stage, re.MULTILINE)
        sources = set()
        for test in (repo / 'resources/scripts').glob('*.test.cjs'):
            sources.update(re.findall(r"['\"]\.\./\.\./([^'\"]+\.(?:go|js))['\"]", test.read_text()))
        # The DOM contract composes these paths from a middleware directory;
        # the literal relative-path scan above cannot discover that form.
        sources.update({
            'infrastructure/core/middlewares/browser-recovery.go',
            'infrastructure/core/middlewares/browser-recovery.css',
        })
        self.assertGreaterEqual(len(sources), 3, 'resource recovery contract inputs disappeared')
        for source in sources:
            with self.subTest(source=source):
                self.assertTrue((repo / source).is_file())
                expected = '/src/' + source
                self.assertTrue(any(
                    source == origin and (destination + Path(source).name if destination.endswith('/') else destination) == expected
                    for origin, destination in copies
                ), f'assets stage does not copy server contract input {source}')

    def test_only_selected_services_and_isolated_storage(self):
        config = smoke.spec('goadmin-smoke-test', '18768', '', Path('/candidate-snapshot'))
        self.assertEqual(set(config['services']), {'admin', 'postgres'})
        admin = config['services']['admin']
        self.assertEqual(admin['environment']['ADMIN_MODULES'], 'users,access,jobs,uploads,queues,notifications,realtime')
        self.assertEqual(admin['build']['args']['STARTER_BUILD_TAGS'], 'starter_smoke')
        self.assertEqual(admin['ports'], ['127.0.0.1:18768:8080'])
        self.assertEqual(config['volumes'], {'postgres_data': {}, 'uploads': {}})
        self.assertEqual(admin['build']['args']['VITE_ADMIN_EXTENSION_MANIFESTS'], '')
        self.assertEqual(admin['build']['additional_contexts']['goauth'], '/candidate-snapshot/goauth')
        self.assertNotIn('env_file', admin)
        self.assertNotIn('external', json.dumps(config['volumes']))

    def test_secret_is_created_private_and_never_overwritten(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'secret.env'
            smoke.protected_write(path, 'secret')
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                smoke.protected_write(path, 'replacement')
            self.assertEqual(path.read_text(), 'secret')

    def test_cleanup_refuses_unowned_run_or_project(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(smoke, 'ARTIFACTS', Path(directory)):
            for invalid in ('', '../starter', 'starter', '20261022t120000z-1234abcd/..'):
                with self.assertRaises(ValueError):
                    smoke.manifest_path(invalid)
            run_id = '20261022t120000z-1234abcd'
            path = smoke.manifest_path(run_id)
            path.parent.mkdir()
            data = {'owner': smoke.MARKER, 'runId': run_id, 'cases': {'default': {
                'project': 'existing-starter', 'env': str(path.parent / 'default.env'),
                'compose': str(path.parent / 'default.compose.json')}}}
            path.write_text(json.dumps(data))
            with self.assertRaises(ValueError):
                smoke.read_manifest(run_id)
            data['cases']['default']['project'] = f'goadmin-smoke-{run_id}-default'
            Path(data['cases']['default']['compose']).write_text(json.dumps({
                'name': data['cases']['default']['project'], 'volumes': {'postgres_data': {}, 'uploads': {}}}))
            path.write_text(json.dumps(data))
            self.assertEqual(smoke.read_manifest(run_id), data)
            data['owner'] = 'another-task'
            path.write_text(json.dumps(data))
            with self.assertRaises(ValueError):
                smoke.read_manifest(run_id)

    def test_candidate_snapshot_excludes_ignored_and_deleted_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'source'; source.mkdir()
            subprocess.run(['git', 'init', '-q', str(source)], check=True)
            (source / '.gitignore').write_text('.env\ncache/\n')
            (source / 'tracked').write_text('initial')
            (source / 'deleted').write_text('removed')
            subprocess.run(['git', '-C', str(source), 'add', '.'], check=True)
            (source / 'tracked').write_text('working change')
            (source / 'deleted').unlink()
            (source / 'new').write_text('new source')
            (source / '.env').write_text('never copied')
            (source / 'cache').mkdir(); (source / 'cache' / 'data').write_text('cache')
            target = root / 'snapshot'
            smoke.snapshot(source, target)
            self.assertEqual((target / 'tracked').read_text(), 'working change')
            self.assertEqual((target / 'new').read_text(), 'new source')
            self.assertFalse((target / '.env').exists())
            self.assertFalse((target / 'cache').exists())
            self.assertFalse((target / 'deleted').exists())
            self.assertFalse((target / '.git').exists())


if __name__ == '__main__':
    unittest.main()
