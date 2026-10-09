"""Reviewed .12 host helpers. Importing this module performs no I/O.

Only additive image task schema; no Canvas replacement, production stop or restart.
All release writers must cooperate on RELEASE_LOCK, including an external updater.
An interrupted mutation is deliberately NOT replayed; inspect its saved phase.
"""
import contextlib
import datetime
import gzip
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import time
import urllib.error
import urllib.request

BASE = Path('/opt/kitty-recovery-20261009/image-upstream-release-r2')
VERSION = '2.9.9-kitty.14'
OLD_VERSION = '2.9.9-kitty.13'
RELEASE_ID = 'image-upstream-20261009'
SITES = ('kittyapi', 'kittyyyds')
PG = {'kittyapi': 'kittyapi-final-c2-20260929t013914z-postgres', 'kittyyyds': 'kittyyyds-postgres'}
CANVAS = {s: s + '-canvas-0-8-0-kitty2-20261009' for s in SITES}
MAIN = Path('/opt/kitty-migration/caddy/Caddyfile')
BACKEND = MAIN.parent / 'backend-router.caddy'
ROUTING = Path('/opt/kitty-updater/state/routing.json')
RELEASE_LOCK = Path('/opt/kitty-updater/state/kitty-release.lock')


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def read_json(path):
    return json.loads(Path(path).read_text(encoding='utf-8-sig'))


def digest(data):
    return hashlib.sha256(data).hexdigest()


def sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def run(args, timeout=60, input=None):
    try:
        p = subprocess.run(args, input=input, capture_output=True, text=True, timeout=timeout)
    except (subprocess.TimeoutExpired, OSError):
        raise RuntimeError('Release command unavailable or timed out; reconcile before retry') from None
    # Never echo arguments, environment, stderr, or container inspection data.
    require(p.returncode == 0, 'Release command failed (' + str(p.returncode) + ')')
    return p.stdout.strip()


def atomic(path, data, mode=0o600, owner=None):
    path = Path(path)
    require(not path.is_symlink(), 'Refusing to replace a symlink')
    data = data.encode() if isinstance(data, str) else data
    temp = path.with_name(path.name + '.plaza-' + str(os.getpid()) + '.tmp')
    with temp.open('xb') as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    os.chmod(temp, mode)
    if owner is not None:
        os.chown(temp, owner, owner)
    os.replace(temp, path)
    fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def save(path, data):
    atomic(path, json.dumps(data, indent=2) + '\n')


@contextlib.contextmanager
def release_lock():
    import fcntl
    require(os.name == 'posix', 'Host helper requires Linux')
    with RELEASE_LOCK.open('a') as stream:
        try:
            fcntl.flock(stream.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('Another release holds the shared host lock') from None
        try:
            yield
        finally:
            fcntl.flock(stream.fileno(), fcntl.LOCK_UN)


def query(site, sql):
    require(site in SITES, 'Unknown site')
    body = "BEGIN READ ONLY;SET LOCAL statement_timeout='5s';SET LOCAL lock_timeout='750ms';SET LOCAL idle_in_transaction_session_timeout='10s';" + sql + ';ROLLBACK;'
    # Credentials stay inside the existing PostgreSQL container.
    command = 'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
    return json.loads(run(['docker', 'exec', '-i', PG[site], 'sh', '-c', command], input=body))


LEDGER_SQL = "SELECT COALESCE(json_object_agg(filename,checksum),'{}') FROM schema_migrations"
# Ignore only the automatic observation timestamp; all actual price, cost,
# capability and operator configuration fields remain protected.
PRICE_FIELDS_SQL = "CASE WHEN g.platform='video' AND jsonb_typeof(g.model_pricing)='array' THEN (SELECT COALESCE(jsonb_agg(CASE WHEN jsonb_typeof(p->'video')='object' THEN jsonb_set(p,'{video}',(p->'video')-'synced_at') ELSE p END ORDER BY n),'[]'::jsonb) FROM jsonb_array_elements(g.model_pricing) WITH ORDINALITY e(p,n)) ELSE g.model_pricing END"
PRICING_SQL = "SELECT to_json(md5(COALESCE(json_agg(t ORDER BY id)::text,'[]'))) FROM (SELECT id," + PRICE_FIELDS_SQL + " AS model_pricing FROM groups g WHERE platform='video' AND deleted_at IS NULL) t"


def sql_digest(select):
    return "SELECT to_json(encode(sha256(convert_to(COALESCE(jsonb_agg(t)::text,'[]'),'UTF8')),'hex')) FROM (" + select + ") t"


# Hash inside PostgreSQL: no encrypted plugin config or setting value leaves DB.
PROTECTED_SQL = {
    'groups_sha256': sql_digest("SELECT (to_jsonb(g)-'updated_at'-'created_at') || jsonb_build_object('model_pricing'," + PRICE_FIELDS_SQL + ") AS config FROM groups g ORDER BY id"),
    'settings_sha256': sql_digest("SELECT key,value FROM settings WHERE key IN ('channel_monitor_mode','channel_monitor_enabled','payment_enabled','payment_balance_disabled','subscription_enabled','site_name','plugin_management_enabled','kitty_video_runtime_v1') OR key LIKE 'model_plaza_%' ORDER BY key"),
    'plugins_sha256': sql_digest("SELECT id,plugin_key,version,state,signature_status,config_encrypted FROM sub2api_plugin_installations ORDER BY id"),
}


def protected(site):
    return {key: query(site, sql) for key, sql in PROTECTED_SQL.items()}


def data_guard(site, contract):
    before = contract['preflight']['sites'][site]
    current = query(site, LEDGER_SQL)
    expected = dict(before['migrations'])
    migration = BASE/'269_kitty_image_supplier_tasks.sql'
    expected[migration.name] = digest(migration.read_bytes().strip())
    require(current == before['migrations'] or current == expected, 'Unexpected production migration ledger')
    require(query(site, PRICING_SQL) == before['video_price_digest'], 'Existing group price digest changed; reconcile before continuing')
    require(protected(site) == before['protected'], 'Group, plugin or protected setting configuration changed; reconcile')
    return current


def contract():
    manifest = read_json(BASE / 'build-manifest.json')
    preflight = read_json(BASE / 'preflight.json')
    backup = read_json(BASE / 'recovery/manifest.json')
    previous = read_json(BASE / 'previous-build-manifest.json')
    require(manifest['version'] == VERSION and manifest.get('completed_at'), 'Incomplete .12 build')
    require(preflight.get('release_id') == RELEASE_ID and preflight.get('target_version') == VERSION, 'Not this release preflight')
    require(manifest.get('release_id') == RELEASE_ID, 'Wrong release identity')
    require(manifest['previous_manifest_sha256'] == preflight['previous_manifest_sha256'] == sha(BASE / 'previous-build-manifest.json'), 'Previous manifest changed')
    require(manifest['preflight_sha256'] == sha(BASE / 'build-preflight.json'), 'Build preflight bytes changed')
    require(read_json(BASE / 'build-preflight.json') == preflight, 'Build and host preflight content differ')
    require(manifest.get('added_migrations') == ['269_kitty_image_supplier_tasks.sql'] and manifest.get('canvas_changed') is False, 'Wrong release scope')
    require(previous['version'] == OLD_VERSION and previous['commit'] == manifest['previous_commit'], 'Wrong .11 baseline')
    require(backup.get('completed_at'), 'Both new backups must be complete')
    require(backup.get('release_id') == RELEASE_ID and backup.get('preflight_sha256') == sha(BASE / 'preflight.json'), 'Backup belongs to another preflight')
    verified = read_json(BASE / 'recovery/verified.json')
    require(verified.get('all_files_sha256_verified') and verified['manifest_sha256'] == sha(BASE / 'recovery/manifest.json'), 'Full recovery set not verified')
    for site in SITES:
        old = preflight['sites'][site]
        require(old['name'] == site + '-2-9-9-kitty.13-20261009', 'Wrong baseline instance')
        require(len(old['migrations']) == 362 and old['binary_sha256'] == previous['sites'][site]['sha256'], 'Baseline identity mismatch')
        require(backup['sites'][site]['postgres_readable'] and backup['sites'][site]['redis_readable'], 'Backup validation missing')
        for name in [site + '-migration-ledger.json', site + '-web-assets.json', site + '-new-web-assets.json', site + '-legacy-assets.json']:
            require(sha(BASE / name) == manifest['sites'][site]['artifact_hashes'][name], 'Artifact digest mismatch: ' + name)
        require(read_json(BASE / (site + '-migration-ledger.json')) == old['migrations'], 'Build migration ledger mismatch')
        require(sha(BASE/'269_kitty_image_supplier_tasks.sql')==manifest['sites'][site]['artifact_hashes']['269_kitty_image_supplier_tasks.sql'],'Image schema bytes changed')
        schema = BASE / 'recovery' / site / 'schema.sql'
        require(sha(schema) == backup['sites'][site]['files']['schema.sql']['sha256'], 'Backup schema changed')
        oldbin = BASE / 'recovery' / site / 'previous-sub2api'
        require(sha(oldbin) == old['binary_sha256'] == backup['sites'][site]['files']['previous-sub2api']['sha256'], 'Backed-up .11 binary mismatch')
        private_metadata = BASE / 'recovery' / site / 'containers.private.json'
        require(sha(private_metadata) == backup['sites'][site]['files']['containers.private.json']['sha256'], 'Fixture image metadata changed')
    return {'manifest': manifest, 'preflight': preflight, 'backup': backup, 'previous': previous,
            'identity': {name: sha(BASE / name) for name in ['build-manifest.json', 'build-preflight.json', 'preflight.json', 'recovery/manifest.json', 'recovery/verified.json', 'previous-build-manifest.json']}}


def require_linux(c):
    evidence = read_json(BASE / 'linux-acceptance.json')
    require(evidence.get('passed') and evidence['identity'] == c['identity'], 'Missing matching Linux acceptance')
    require(evidence.get('tool_sha256') == sha(BASE / 'linux_acceptance.py'), 'Linux test helper changed after acceptance')
    require(evidence.get('common_sha256') == sha(BASE / 'release_common.py'), 'Common helper changed after acceptance')
    require(evidence.get('image_test_sha256') == sha(BASE / 'image_acceptance.py'), 'Image test helper changed after acceptance')
    require(set(evidence.get('sites', {})) == set(SITES), 'Both sites require matching Linux acceptance')


def inspect(name):
    return json.loads(run(['docker', 'inspect', name]))[0]


class DockerConnection(http.client.HTTPConnection):
    def __init__(self):
        super().__init__('localhost', timeout=90)

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect('/var/run/docker.sock')


def docker_api(method, path, data=None):
    c = DockerConnection()
    try:
        c.request(method, path, body=json.dumps(data) if data is not None else None, headers={'Content-Type': 'application/json'})
        result = c.getresponse()
        raw = result.read()
        require(result.status < 300, 'Docker API request failed (' + str(result.status) + ')')
        return json.loads(raw) if raw else None
    finally:
        c.close()


def clone(old, name, env, mounts):
    cfg = {k: old['Config'][k] for k in ['User', 'WorkingDir', 'Entrypoint', 'Cmd', 'ExposedPorts', 'Healthcheck', 'StopSignal', 'Labels'] if old['Config'].get(k) is not None}
    cfg['Image'] = old['Image']  # Pin the immutable image ID, never a mutable tag.
    cfg['Env'] = [k + '=' + v for k, v in env.items()]
    hc = {k: old['HostConfig'][k] for k in ['ReadonlyRootfs', 'RestartPolicy', 'CapDrop', 'CapAdd', 'SecurityOpt', 'Tmpfs', 'Memory', 'MemorySwap', 'NanoCpus', 'PidsLimit', 'LogConfig', 'Ulimits', 'Dns', 'DnsSearch', 'ExtraHosts', 'GroupAdd', 'Init'] if old['HostConfig'].get(k) is not None}
    hc['NetworkMode'] = old['HostConfig']['NetworkMode']
    require(hc['NetworkMode'] in old['NetworkSettings']['Networks'], 'Unsupported container network mode')
    hc['Binds'] = mounts
    if not hc.get('Memory'):
        hc.update(Memory=4 * 1024**3, MemorySwap=4 * 1024**3)
    if not hc.get('NanoCpus'):
        hc['NanoCpus'] = 4_000_000_000
    cfg['HostConfig'] = hc
    docker_api('POST', '/containers/create?name=' + name, cfg)
    for network in old['NetworkSettings']['Networks']:
        if network != hc['NetworkMode']:
            docker_api('POST', '/networks/' + network + '/connect', {'Container': name})
    docker_api('POST', '/containers/' + name + '/start')
    return inspect(name)['NetworkSettings']['Networks'][hc['NetworkMode']]['IPAddress']


def fetch(url, host=None):
    headers = {'User-Agent': 'Kitty-pricing-release-check', 'Cache-Control': 'no-cache', 'Accept-Encoding': 'identity'}
    if host:
        headers['Host'] = host
    # Do not inherit host HTTP proxy settings for private-IP readiness checks.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(urllib.request.Request(url, headers=headers), timeout=20) as response:
            return response.status, response.read(16 * 1024 * 1024), dict(response.headers)
    except urllib.error.HTTPError as error:
        return error.code, error.read(8192), dict(error.headers)


def wait(fn, seconds, label):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            value = fn()
            if value:
                return value
        except (OSError, ValueError, RuntimeError, KeyError):
            pass
        time.sleep(1)
    raise RuntimeError(label + ' timed out; reconcile saved state, do not replay')


def status(role_file, instance):
    value = read_json(Path(role_file).parent / 'status.json')
    age = datetime.datetime.now(datetime.timezone.utc) - datetime.datetime.fromisoformat(value['updated_at'].replace('Z', '+00:00'))
    require(value['instance_id'] == instance and 0 <= age.total_seconds() < 20, 'Stale or mismatched managed status')
    return value


def ready(role_file, name, expected_role, leader=None):
    s = status(role_file, name)
    ok = s['role'] == expected_role and s['ready'] and s['accounting_ready'] and s['lease_healthy'] and not s.get('error')
    if leader is not None:
        ok = ok and s['leadership_owned'] is leader
    return s if ok else None


def mapped_mount(row, path):
    matches = [m for m in row['Mounts'] if path.startswith(m['Destination'].rstrip('/') + '/')]
    require(bool(matches), 'Managed runtime path is not directory-mounted')
    return max(matches, key=lambda m: len(m['Destination']))


def role_path(row):
    env = dict(x.split('=', 1) for x in row['Config']['Env'] if '=' in x)
    require(env.get('KITTY_MANAGED_INSTANCE') == 'true', 'Source instance is not managed')
    target = env['KITTY_INSTANCE_ROLE_FILE']
    mount = mapped_mount(row, target)
    return Path(mount['Source']) / Path(target).relative_to(mount['Destination'])


def set_role(path, expected, new):
    require((expected, new) in [('standby', 'active'), ('active', 'draining')], 'Forbidden role transition')
    require(Path(path).read_text().strip() == expected, 'Role CAS failed')
    atomic(path, new + '\n', 0o644, 1000)


def step(root, state, phase):
    state['phase'] = phase
    state.setdefault('steps', []).append({'at': now(), 'phase': phase})
    save(root / 'release.json', state)
    print(json.dumps({'site': state['site'], 'phase': phase, 'instance': state['new']}), flush=True)


def unpack_binary(entry):
    binary = BASE / entry['binary'].replace('\\', '/').split('/')[-1]
    if not binary.exists():
        compressed = Path(str(binary) + '.gz')
        require(sha(compressed) == entry['compressed_sha256'], 'Compressed binary digest mismatch')
        temp = binary.with_name(binary.name + '.unpack')
        with gzip.open(compressed, 'rb') as src, temp.open('xb') as dst:
            shutil.copyfileobj(src, dst)
        require(sha(temp) == entry['sha256'], 'Unpacked binary digest mismatch')
        os.chmod(temp, 0o755)
        os.replace(temp, binary)
    require(sha(binary) == entry['sha256'], 'Binary digest mismatch')
    return binary


def canvas_snapshot(site, c):
    row = inspect(CANVAS[site])
    require(row['State']['Running'] and row['Image'] == c['backup']['sites'][site]['canvas_image'], 'Canvas baseline changed')
    result = {'id': row['Id'], 'image': row['Image'], 'restarts': row['RestartCount']}
    require(result == c['preflight']['sites'][site]['canvas'], 'Canvas differs from fresh preflight')
    return result


def initial_routes(c, site):
    require(site in SITES, 'Unknown site')
    expected = dict(c['preflight']['routes_sha256'])
    if site == 'kittyyyds':
        accepted = read_json(BASE / 'acceptance-kittyapi.json')
        require(accepted.get('passed') and accepted['identity'] == c['identity'], 'Accept kittyapi before starting the YYDS release')
        first = read_json(state_root('kittyapi') / 'release.json')
        require(first['phase'] == 'completed' and first['identity'] == c['identity'], 'API deployment is not complete')
        require(ready(first['role_file'], first['new'], 'active', True), 'API leadership unhealthy')
        data_guard('kittyapi', c)
        expected[str(MAIN)] = first['route_after_sha256']
    for path, expected_sha in expected.items():
        require(sha(Path(path)) == expected_sha, 'Routing changed since reviewed stage: ' + Path(path).name)


def state_root(site):
    require(site in SITES, 'Unknown site')
    return BASE / (site + '-v14')


def prepare(site):
    with release_lock():
        c = contract()
        require_linux(c)
        data_guard(site, c)
        initial_routes(c, site)
        root = state_root(site)
        require(not root.exists(), 'Preparation already exists; reconcile instead of repeating')
        oldname = c['preflight']['sites'][site]['name']
        version = VERSION
        entry = c['manifest']['sites'][site]
        expected_old_sha = c['previous']['sites'][site]['sha256']
        binary = unpack_binary(entry)
        old = inspect(oldname)
        require(old['State']['Running'] and not old['State']['OOMKilled'], 'Old instance is not healthy')
        require(old['Id'] == c['preflight']['sites'][site]['container_id'], 'Old container changed since preflight')
        require(run(['docker', 'exec', oldname, 'sha256sum', '/proc/1/exe']).split()[0] == expected_old_sha, 'Old runtime binary mismatch')
        oldrole = role_path(old)
        require(oldrole.read_text().strip() == 'active' and ready(oldrole, oldname, 'active', True), 'Old instance must still be active leader')
        require(old['Image'] == c['preflight']['sites'][site]['image_id'], 'Unexpected runtime image')
        apply_image_schema(site,c)
        root.mkdir(mode=0o700)
        name = site + '-2-9-9-kitty.14-20261009'
        state = {'site': site, 'old': oldname, 'new': name, 'version': version,
                 'identity': c['identity'], 'binary_sha256': entry['sha256'], 'old_binary_sha256': expected_old_sha,
                 'old_role_file': str(oldrole), 'canvas': canvas_snapshot(site, c), 'started_at': now()}
        step(root, state, 'preparing')
        save(root / 'container-before.private.json', old)
        env = dict(x.split('=', 1) for x in old['Config']['Env'] if '=' in x)
        entrypoint = old['Config']['Entrypoint'][0]
        runtime = mapped_mount(old, entrypoint)
        role_mount = mapped_mount(old, env['KITTY_INSTANCE_ROLE_FILE'])
        require(runtime['Source'] != role_mount['Source'], 'Runtime and role mounts must be separate')
        newruntime = root / 'runtime'
        shutil.copytree(runtime['Source'], newruntime, symlinks=True)
        target = newruntime / Path(entrypoint).relative_to(runtime['Destination'])
        require(target.resolve().is_relative_to(newruntime.resolve()), 'Runtime executable escapes copied tree')
        shutil.copy2(binary, target)
        os.chmod(target, 0o755)
        for path in [newruntime, *newruntime.rglob('*')]:
            if not path.is_symlink():
                os.chown(path, 1000, 1000)
        newrole = root / 'role'
        rolefile = newrole / Path(env['KITTY_INSTANCE_ROLE_FILE']).relative_to(role_mount['Destination'])
        rolefile.parent.mkdir(parents=True)
        for path in [newrole, *newrole.rglob('*')]:
            os.chown(path, 1000, 1000)
        atomic(rolefile, 'standby\n', 0o644, 1000)
        env['KITTY_INSTANCE_ID'] = name
        binds = []
        for mount in old['Mounts']:
            source = mount['Source']
            if source == role_mount['Source']:
                source = str(newrole)
            elif source == runtime['Source'] or source.startswith(runtime['Source'] + '/'):
                source = str(newruntime) + source[len(runtime['Source']):]
            mode = mount.get('Mode') or ('rw' if mount['RW'] else 'ro')
            binds.append(source + ':' + mount['Destination'] + ':' + mode)
        state['role_file'] = str(rolefile)
        save(root / 'release.json', state)
        state['ip'] = clone(old, name, env, binds)
        state['container_id'] = inspect(name)['Id']
        save(root / 'release.json', state)
        wait(lambda: ready(rolefile, name, 'standby', False), 180, 'standby readiness')
        code, body, _ = fetch('http://' + state['ip'] + ':8080/api/v1/settings/public', site + '.xyz')
        require(code == 200 and json.loads(body)['data']['version'] == version, 'Candidate public version mismatch')
        require(run(['docker', 'exec', name, 'sha256sum', '/proc/1/exe']).split()[0] == entry['sha256'], 'Candidate executable mismatch')
        for path in ['/api/v1/admin/video-upstream', '/api/v1/admin/image-upstream', '/api/v1/canvas/bootstrap']:
            require(fetch('http://' + state['ip'] + ':8080' + path, site + '.xyz')[0] == 401, 'Candidate auth boundary failed')
        helper = newruntime / 'kitty-plugin-check'
        require(helper.is_file() and sha(helper) == sha(Path(runtime['Source']) / helper.name), 'Plugin helper changed')
        report = next(json.loads(line) for line in run(['docker', 'exec', '--user', '1000:1000', name, str(Path(entrypoint).parent / helper.name)], 120).splitlines() if line.startswith('{'))
        require(report.get('ready') and report.get('ui_status') == 200, 'Plugin readiness failed')
        check_assets(site, state, public=False)
        data_guard(site, c)
        require(canvas_snapshot(site, c) == state['canvas'], 'Canvas changed during prepare')
        step(root, state, 'prepared')


def route_plan(site, old, new, ip):
    require(site in SITES, 'Unknown site')
    before = {MAIN: MAIN.read_bytes(), BACKEND: BACKEND.read_bytes(), ROUTING: ROUTING.read_bytes()}
    require(b'kitty-fenjue-test-app:8080' in before[MAIN], 'Test-site route missing')
    if site == 'kittyapi':
        needle = (old + ':8080').encode()
        require(before[MAIN].count(needle) == 1, 'Expected exactly one old kittyapi upstream')
        after = before[MAIN].replace(needle, (new + ':8080').encode())
        require(after.count((new + ':8080').encode()) == 1, 'Candidate upstream already present')
        return before, MAIN, after
    before[ROUTING] = ROUTING.read_bytes()
    route = json.loads(before[ROUTING])
    source = inspect(old)
    oldip = source['NetworkSettings']['Networks'][source['HostConfig']['NetworkMode']]['IPAddress']
    require(route['instance_id'] == old and route['backend'] == 'http://' + oldip + ':8080', 'YYDS route identity mismatch')
    require(type(route['generation']) is int and route['generation'] >= 0, 'Invalid routing generation')
    route.update(generation=route['generation'] + 1, instance_id=new, backend='http://' + ip + ':8080')
    return before, ROUTING, (json.dumps(route, indent=2) + '\n').encode()


def cas_routes(before, target, after):
    # Compare + rename is serialized by RELEASE_LOCK. Atomic rename alone is NOT
    # a CAS against non-cooperating writers. Deployment requires their quiescence.
    for path, raw in before.items():
        require(path.read_bytes() == raw, 'Routing CAS failed: ' + path.name)
    mode = target.stat().st_mode & 0o777
    atomic(target, after, mode)
    require(target.read_bytes() == after, 'Route write did not persist')


def activate(site):
    with release_lock():
        c = contract()
        require_linux(c)
        initial_routes(c, site)
        root = state_root(site)
        state = read_json(root / 'release.json')
        require(state['phase'] == 'prepared' and state['identity'] == c['identity'], 'Not an unchanged prepared release; do not replay')
        data_guard(site, c)
        require(inspect(state['old'])['Id'] == c['preflight']['sites'][site]['container_id'], 'Old container identity changed')
        oldrole, newrole = Path(state['old_role_file']), Path(state['role_file'])
        require(ready(oldrole, state['old'], 'active', True), 'Old leader changed')
        require(ready(newrole, state['new'], 'standby', False), 'Candidate is not standby')
        candidate = inspect(state['new'])
        require(candidate['State']['Running'] and not candidate['State']['OOMKilled'] and candidate['RestartCount'] == 0, 'Candidate restarted or failed')
        require(candidate['Id'] == state['container_id'], 'Candidate container identity changed')
        require(run(['docker', 'exec', state['new'], 'sha256sum', '/proc/1/exe']).split()[0] == state['binary_sha256'], 'Candidate changed after prepare')
        require(candidate['NetworkSettings']['Networks'][candidate['HostConfig']['NetworkMode']]['IPAddress'] == state['ip'], 'Candidate IP changed')
        require(canvas_snapshot(site, c) == state['canvas'], 'Canvas changed before activation')
        before, target, after = route_plan(site, state['old'], state['new'], state['ip'])
        for path, raw in before.items():
            atomic(root / (path.name + '.before.private'), raw)
        atomic(root / (target.name + '.after.private'), after)
        if site == 'kittyapi':
            pending = MAIN.with_name('Caddyfile.plaza-' + state['new'] + '-pending')
            require(not pending.exists(), 'Candidate config already exists; reconcile')
            atomic(pending, after, 0o644)
            run(['docker', 'exec', 'kitty-migration-caddy', 'caddy', 'validate', '--config', '/etc/caddy/' + pending.name, '--adapter', 'caddyfile'])
        state['route_target'] = str(target)
        state['route_before_sha256'] = digest(before[target])
        state['route_after_sha256'] = digest(after)
        state['backend_router_sha256'] = digest(before[BACKEND])
        # Recheck data immediately before roles/routes change, after validation.
        data_guard(site, c)
        initial_routes(c, site)
        step(root, state, 'switching')  # Persist intent BEFORE any role or routing mutation.
        set_role(newrole, 'standby', 'active')
        wait(lambda: ready(newrole, state['new'], 'active'), 30, 'active role publication')
        cas_routes(before, target, after)
        step(root, state, 'route_written')
        if site == 'kittyapi':
            run(['docker', 'exec', 'kitty-migration-caddy', 'caddy', 'reload', '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile'])
        step(root, state, 'routed')
        set_role(oldrole, 'active', 'draining')
        step(root, state, 'draining_old')

        def handoff():
            old = status(oldrole, state['old'])
            new = ready(newrole, state['new'], 'active', True)
            return new if new and old['role'] == 'draining' and old['leadership_released'] and not old['leadership_owned'] else None

        wait(handoff, 300, 'background leadership handoff')
        for host in [site + '.xyz', 'cdn.' + site + '.xyz']:
            def public():
                code, body, _ = fetch('https://' + host + '/api/v1/settings/public')
                return code == 200 and json.loads(body)['data']['version'] == state['version']
            wait(public, 120, 'public version')
        data_guard(site, c)
        require(sha(target) == state['route_after_sha256'], 'Route changed after activation')
        require(sha(BACKEND) == state['backend_router_sha256'], 'Canvas/router config changed')
        require(canvas_snapshot(site, c) == state['canvas'], 'Canvas changed during activation')
        require(inspect(state['old'])['State']['Running'], 'Old instance must remain running')
        state['old_stopped'] = False
        step(root, state, 'completed')


def check_assets(site, state, public=True):
    assets = read_json(BASE / (site + '-web-assets.json'))
    new = read_json(BASE / (site + '-new-web-assets.json'))
    legacy = read_json(BASE / (site + '-legacy-assets.json'))
    targets = ['https://' + site + '.xyz', 'https://cdn.' + site + '.xyz'] if public else ['http://' + state['ip'] + ':8080']
    for origin in targets:
        host = None if public else site + '.xyz'
        code, html, _ = fetch(origin + '/admin/video-upstream', host)
        require(code == 200, 'Frontend document unavailable')
        paths = {p.decode().lstrip('/') for p in re.findall(rb'<script[^>]+src="(/assets/[^"]+)"', html)}
        require(bool(paths), 'Frontend entry assets missing')
        paths.update(p for p in new if p.startswith('assets/') and any(k in p for k in ['ModelPlazaView-', 'CreativeCanvasView-', 'VideoUpstreamView-', 'ImageUpstreamPanel-']))
        paths.update(sorted(legacy)[:3])
        for path in sorted(paths):
            require(path.startswith('assets/') and '..' not in path.split('/'), 'Unsafe asset path')
            code, body, _ = fetch(origin + '/' + path, host)
            require(code == 200 and digest(body) == assets[path], 'Static resource mismatch: ' + path)


def accept(sites=SITES):
    with release_lock():
        c = contract()
        require_linux(c)
        out = {'at': now(), 'identity': c['identity'], 'sites': {}, 'passed': False, 'paid_requests': 0}
        for site in sites:
            state = read_json(state_root(site) / 'release.json')
            require(state['phase'] == 'completed' and state['identity'] == c['identity'], 'Incomplete or mismatched deployment')
            row = inspect(state['new'])
            require(row['Id'] == state['container_id'], 'Accepted container differs from prepared candidate')
            require(row['State']['Running'] and not row['State']['OOMKilled'] and row['RestartCount'] == 0, 'New runtime is unhealthy')
            require(run(['docker', 'exec', state['new'], 'sha256sum', '/proc/1/exe']).split()[0] == state['binary_sha256'], 'Runtime binary changed')
            new = ready(state['role_file'], state['new'], 'active', True)
            require(new, 'New leadership unhealthy')
            old = status(state['old_role_file'], state['old'])
            require(old['role'] == 'draining' and old['leadership_released'] and not old['leadership_owned'], 'Old leadership not released')
            require(inspect(state['old'])['State']['Running'], 'Old instance was stopped')
            data_guard(site, c)
            require(sha(Path(state['route_target'])) == state['route_after_sha256'], 'Deployment route changed')
            require(sha(BACKEND) == state['backend_router_sha256'], 'Canvas routing changed')
            require(canvas_snapshot(site, c) == state['canvas'], 'Canvas runtime changed')
            check_assets(site, state)
            for host in [site + '.xyz', 'cdn.' + site + '.xyz']:
                code, body, _ = fetch('https://' + host + '/api/v1/settings/public')
                require(code == 200 and json.loads(body)['data']['version'] == state['version'], 'Public version mismatch')
                for path in ['/health', '/model-plaza', '/admin/groups', '/admin/video-upstream', '/creative-canvas', '/video-tasks']:
                    require(fetch('https://' + host + path)[0] == 200, 'Public page unavailable')
                for path in ['/api/v1/admin/video-upstream', '/api/v1/admin/image-upstream', '/api/v1/canvas/bootstrap']:
                    require(fetch('https://' + host + path)[0] == 401, 'Public auth boundary failed')
                code, _, headers = fetch('https://' + host + '/canvas/canvas?kitty_version=' + state['version'])
                require(code == 200 and 'no-store' in headers.get('Cache-Control', ''), 'Canvas document/cache regression')
            logs = subprocess.run(['docker', 'logs', '--since', row['State']['StartedAt'], state['new']], capture_output=True, text=True, timeout=30)
            require(logs.returncode == 0, 'Unable to inspect runtime logs')
            require(not re.search(r'(?im)panic:|level=fatal|migration failed|checksum mismatch|plugin_(?:initial_)?reconcile_failed', logs.stdout + logs.stderr), 'Runtime failure marker detected')
            out['sites'][site] = {'version': state['version'], 'binary_sha256': state['binary_sha256'], 'schema_count': 363,
                'price_digest_unchanged': True, 'leader_healthy': True, 'canvas_unchanged': True,
                'old_running': True, 'old_active_requests': old['active_requests'], 'old_active_ws': old['active_ws'], 'old_retire_ready': old['retire_ready']}
        require(b'kitty-fenjue-test-app:8080' in MAIN.read_bytes(), 'Test route disappeared')
        require(fetch('https://cs.kittyyyds.xyz/health')[0] == 200, 'Test site unavailable')
        out['passed'] = True
        save(BASE / ('acceptance-' + '-'.join(sites) + '.json'), out)
        print(json.dumps(out), flush=True)


def apply_image_schema(site,c):
    migration=BASE/'269_kitty_image_supplier_tasks.sql'
    text=migration.read_text().strip();checksum=digest(text.encode())
    expected=dict(c['preflight']['sites'][site]['migrations']);expected[migration.name]=checksum
    current=query(site,LEDGER_SQL)
    if current==expected:return
    require(current==c['preflight']['sites'][site]['migrations'],'Schema changed before additive migration')
    require('image_supplier_tasks' in text and 'DROP ' not in text and 'ALTER ' not in text,'Unexpected migration operations')
    statement="BEGIN;SET LOCAL lock_timeout='750ms';SET LOCAL statement_timeout='15s';DO $$BEGIN IF NOT pg_try_advisory_xact_lock(694208311321144027) THEN RAISE EXCEPTION 'migration busy'; END IF;END$$;"+text+"INSERT INTO schema_migrations(filename,checksum) VALUES ('269_kitty_image_supplier_tasks.sql','"+checksum+"');COMMIT;"
    command='PGPASSWORD="$POSTGRES_PASSWORD" exec psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
    run(['docker','exec','-i',PG[site],'sh','-c',command],input=statement)
    require(query(site,LEDGER_SQL)==expected,'New schema verification failed')
