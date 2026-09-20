#!/usr/bin/env python3
import os
import json
import re
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import threading
import time
import sys
import urllib.parse
import urllib.request
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

root = Path(__file__).resolve().parent.parent
env = dict(os.environ)
base = 'http://127.0.0.1:8081'
retain_demo = env.get('REFORGE_KEEP_DEMO') == '1'

def process_starttime(pid):
    fields = Path(f'/proc/{pid}/stat').read_text().split()
    return fields[21]

def validate_process(pid, expected_starttime, executable, required_arg):
    if not isinstance(pid, int) or pid <= 1 or not isinstance(expected_starttime, str):
        return False
    try:
        actual = process_starttime(pid)
        command = Path(f'/proc/{pid}/exe').resolve()
        cmdline = Path(f'/proc/{pid}/cmdline').read_bytes().replace(b'\0', b' ').decode()
        environment = Path(f'/proc/{pid}/environ').read_bytes().replace(b'\0', b' ').decode()
    except (FileNotFoundError, OSError, ValueError):
        return False
    return actual == expected_starttime and command == root / executable and (required_arg in cmdline or required_arg in environment)

if '--cleanup-demo' in sys.argv:
    manifest_path = root / '.local/demo-repair.json'
    data = json.loads(manifest_path.read_text())
    db = data.get('database', '')
    repo = data.get('native_full_name', '')
    fixture = data.get('fixture_directory', '')
    if not re.fullmatch(r'reforge_browser_[0-9a-f]{32}', db):
        raise SystemExit('refusing unexpected demo database')
    if not re.fullmatch(r'reforge-bot/browser-repair-[0-9]+', repo):
        raise SystemExit('refusing unexpected fixture repository')
    fixture_path = Path(fixture)
    if fixture_path.parent != Path('/tmp') or not re.fullmatch(r'reforge-repair-fixture-[A-Za-z0-9]+', fixture_path.name):
        raise SystemExit('refusing unexpected fixture directory')
    controller_pid = data.get('controller_pid')
    if Path(f'/proc/{controller_pid}').exists() and not validate_process(controller_pid, data.get('controller_starttime'), 'bin/reforge', 'REFORGE_ADDRESS=127.0.0.1:8081'):
        raise SystemExit('refusing unexpected controller PID')
    runner_pid = data.get('runner_pid')
    if isinstance(runner_pid, int) and Path(f'/proc/{runner_pid}').exists() and not validate_process(runner_pid, data.get('runner_starttime'), 'bin/reforge-runner', '127.0.0.1:8081'):
        raise SystemExit('refusing unexpected runner PID')
    credential = Path(data.get('runner_credentials', ''))
    if credential != fixture_path / 'runner-credentials':
        raise SystemExit('refusing unexpected runner credential path')
    if Path(f'/proc/{controller_pid}').exists():
        os.kill(controller_pid, signal.SIGTERM)
    if isinstance(runner_pid, int) and Path(f'/proc/{runner_pid}').exists():
        os.kill(runner_pid, signal.SIGTERM)
    admin = dict(env, PGHOST='127.0.0.1', PGPORT='55432', PGUSER='mnorris', PGDATABASE='postgres', PGPASSWORD=(root / '.local/pg-admin-password').read_text().strip())
    subprocess.run(['psql', '-X', '-w', '-v', 'ON_ERROR_STOP=1', '-c', 'DROP DATABASE ' + db + ' WITH (FORCE)'], env=admin, check=True, stdout=subprocess.DEVNULL)
    token = (root / '.local/gitea/reforge-admin.token').read_text().strip()
    request = urllib.request.Request('http://127.0.0.1:53000/api/v1/repos/' + repo, method='DELETE', headers={'Authorization': 'token ' + token})
    with urllib.request.urlopen(request, timeout=10):
        pass
    shutil.rmtree(fixture_path)
    shutil.rmtree(root / '.local/repair-demo')
    manifest_path.unlink()
    raise SystemExit(0)
if retain_demo and env.get('REFORGE_CAPTURE_MODEL') == '1':
    raise SystemExit('REFORGE_CAPTURE_MODEL cannot be combined with REFORGE_KEEP_DEMO')
for name in ('REFORGE_DATABASE_URL', 'REFORGE_MIGRATION_DATABASE_URL'):
    parsed = urllib.parse.urlsplit(env.get(name, ''))
    if parsed.hostname != '127.0.0.1' or parsed.port != 55432:
        raise SystemExit('Requires disposable local PostgreSQL URLs on 127.0.0.1:55432')
if not env.get('REFORGE_BROWSER_RUNTIME_CONFIG'):
    raise SystemExit('Set REFORGE_BROWSER_RUNTIME_CONFIG to the verified development runtime configuration')
try:
    images = json.loads(env.get('REFORGE_REPAIR_IMAGES', ''))
except json.JSONDecodeError as error:
    raise SystemExit('Set REFORGE_REPAIR_IMAGES to nonempty JSON with a Go image digest') from error
go_image = images.get('go') if isinstance(images, dict) else None
if not isinstance(go_image, str) or len(go_image) != 71 or not go_image.startswith('sha256:') or any(char not in '0123456789abcdef' for char in go_image[7:]):
    raise SystemExit('REFORGE_REPAIR_IMAGES.go must be a sha256 digest')
try:
    with urllib.request.urlopen(base + '/healthz', timeout=1):
        raise SystemExit('Port 8081 already has a controller; refusing shared-state execution')
except (OSError, urllib.error.URLError):
    pass
name = 'reforge_browser_' + uuid.uuid4().hex
admin = dict(env, PGHOST='127.0.0.1', PGPORT='55432', PGUSER='mnorris', PGDATABASE='postgres', PGPASSWORD=(root / '.local/pg-admin-password').read_text().strip())
if retain_demo:
    work = root / '.local/repair-demo'
    if work.exists():
        raise SystemExit(f'{work} already exists; remove the prior isolated demo before retaining another')
    work.mkdir(mode=0o700, parents=True)
else:
    work = Path(tempfile.mkdtemp(prefix='reforge-browser-'))
server = None
proxy = None
created = False
result = 1
log = None
browser = None
proxy_thread = None

class ModelCaptureProxy(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    upstream = 'http://127.0.0.1:55435'
    capture_limit = 1024 * 1024
    capture_directory = None

    def log_message(self, format, *args):
        return

    def do_GET(self):
        self.forward(None)

    def do_POST(self):
        length = int(self.headers.get('Content-Length', '0'))
        body = self.rfile.read(length)
        self.forward(body)

    def do_PUT(self):
        self.do_POST()

    def do_DELETE(self):
        self.forward(None)

    def capture_request(self, body):
        if self.path != '/v1/chat/completions' or not body or self.headers.get_content_type() != 'application/json':
            return
        if len(body) > self.capture_limit:
            body = body[:self.capture_limit]
        capture_id = uuid.uuid4().hex
        path = self.capture_directory / f'request-{capture_id}.json'
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        try:
            with os.fdopen(descriptor, 'wb') as output:
                output.write(body)
        finally:
            os.chmod(path, 0o600)
        return capture_id

    def forward(self, body):
        capture_id = self.capture_request(body)
        target = self.upstream + self.path
        request = urllib.request.Request(target, data=body, method=self.command)
        for key, value in self.headers.items():
            if key.lower() not in {'host', 'content-length', 'connection'}:
                request.add_header(key, value)
        try:
            response = urllib.request.urlopen(request, timeout=180)
        except urllib.error.HTTPError as error:
            response = error
        except OSError as error:
            self.send_error(502, str(error))
            return
        with response:
            self.send_response(response.status)
            for key, value in response.headers.items():
                if key.lower() not in {'connection', 'content-length', 'transfer-encoding'}:
                    self.send_header(key, value)
            self.send_header('Connection', 'close')
            self.end_headers()
            self.close_connection = True
            capture = capture_id is not None and response.headers.get_content_type() == 'text/event-stream'
            captured = bytearray()
            while True:
                chunk = response.read1(64 * 1024)
                if not chunk:
                    break
                self.wfile.write(chunk)
                self.wfile.flush()
                if capture and len(captured) < self.capture_limit:
                    captured.extend(chunk[:self.capture_limit - len(captured)])
            if capture and captured:
                path = self.capture_directory / f'response-{capture_id}.sse'
                descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                try:
                    with os.fdopen(descriptor, 'wb') as output:
                        output.write(captured)
                finally:
                    os.chmod(path, 0o600)

def stop_process(process):
    if process is None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=20)
    except subprocess.TimeoutExpired:
        pass
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        return
    process.wait(timeout=5)

try:
    subprocess.run(['psql', '-X', '-w', '-v', 'ON_ERROR_STOP=1', '-c', 'CREATE DATABASE ' + name + ' OWNER reforge_migrator'], env=admin, check=True, stdout=subprocess.DEVNULL)
    created = True
    for key in ('REFORGE_DATABASE_URL', 'REFORGE_MIGRATION_DATABASE_URL'):
        parsed = urllib.parse.urlsplit(env[key])
        env[key] = urllib.parse.urlunsplit(parsed._replace(path='/' + name))
    subprocess.run([str(root / 'bin/reforge-migrate')], cwd=root, env=env, check=True)
    grants = dict(admin, PGDATABASE=name)
    subprocess.run(['psql', '-X', '-w', '-v', 'ON_ERROR_STOP=1', '-f', str(root / 'scripts/runtime-grants.sql')], env=grants, check=True, stdout=subprocess.DEVNULL)
    env.update(REFORGE_MODE='development', REFORGE_FIXTURE_AUTH='true', REFORGE_EDITION='self-hosted', REFORGE_ADDRESS='127.0.0.1:8081', REFORGE_PUBLIC_URL=base, REFORGE_BASE_URL=base, REFORGE_ARTIFACT_DIRECTORY=str(work / 'artifacts'), REFORGE_ISOLATED_BROWSER_DATABASE='1', REFORGE_LIVE_REPAIR_BROWSER='1')
    if retain_demo:
        env['REFORGE_DEMO_MANIFEST'] = str(root / '.local/demo-repair.json')
    if env.get('REFORGE_CAPTURE_MODEL') == '1':
        capture_parent = root / '.local/model-capture'
        capture_parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        os.chmod(capture_parent, 0o700)
        ModelCaptureProxy.capture_directory = capture_parent / f'browser-{uuid.uuid4().hex}'
        ModelCaptureProxy.capture_directory.mkdir(mode=0o700)
        proxy = ThreadingHTTPServer(('127.0.0.1', 0), ModelCaptureProxy)
        proxy.daemon_threads = True
        proxy_thread = threading.Thread(target=proxy.serve_forever, daemon=True)
        proxy_thread.start()
        env['REFORGE_BROWSER_MODEL_ENDPOINT'] = f'http://127.0.0.1:{proxy.server_address[1]}'
    log = (work / 'controller.log').open('w')
    server = subprocess.Popen([str(root / 'bin/reforge')], cwd=root, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
    for attempt in range(100):
        if server.poll() is not None:
            raise RuntimeError('Disposable controller exited before readiness')
        try:
            with urllib.request.urlopen(base + '/readyz', timeout=1) as response:
                if response.status == 200:
                    break
        except (OSError, urllib.error.URLError):
            time.sleep(0.1)
    else:
        raise RuntimeError('Disposable controller did not become ready')
    if retain_demo:
        manifest = root / '.local/demo-repair.json'
        manifest.write_text(json.dumps({
            'controller_url': base,
            'controller_pid': server.pid,
            'controller_starttime': process_starttime(server.pid),
            'database': name,
            'work_directory': str(work),
            'status': 'running',
        }, indent=2) + '\n')
        os.chmod(manifest, 0o600)
    browser = subprocess.Popen(['npx', 'playwright', 'test', 'tests/repair-live.spec.ts', '--workers=1'], cwd=root / 'web', env=env, start_new_session=True)
    try:
        result = browser.wait(timeout=480)
    except subprocess.TimeoutExpired as error:
        stop_process(browser)
        raise RuntimeError('Browser repair test timed out') from error
finally:
    stop_process(browser)
    if not retain_demo:
        stop_process(server)
    if proxy is not None:
        proxy.shutdown()
        proxy.server_close()
        if proxy_thread is not None:
            proxy_thread.join(timeout=5)
    if log is not None:
        log.close()
        shutil.copyfile(work / 'controller.log', root / '.local/repair-browser-controller.log')
    if retain_demo and result != 0:
        manifest = root / '.local/demo-repair.json'
        if manifest.exists():
            data = json.loads(manifest.read_text())
            data['status'] = 'failed'
            manifest.write_text(json.dumps(data, indent=2) + '\n')
            os.chmod(manifest, 0o600)
    if created and not retain_demo:
        subprocess.run(['psql', '-X', '-w', '-v', 'ON_ERROR_STOP=1', '-c', 'DROP DATABASE ' + name + ' WITH (FORCE)'], env=admin, check=True, stdout=subprocess.DEVNULL)
    if not retain_demo:
        shutil.rmtree(work)
raise SystemExit(result)
