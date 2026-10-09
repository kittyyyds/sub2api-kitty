"""Linux schema362 + synthetic HTTP checks on a Docker --internal network.

No production data connection, public network, generation, or charging request.
Only containers created by this run, carrying its exact label, may be stopped.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import secrets
import sys
import urllib.error
import urllib.request

sys.dont_write_bytecode = True
sys.path.insert(0, '/opt/kitty-recovery-20261009/video-plaza')
import release_common as rc

MODEL = '全能sd2.0(满血原生过脸-933-480p)'
FIXTURE = {'success': True, 'pricing_version': 'isolated-v12', 'group_ratio': {'default': 1}, 'data': [
    {'model_name': MODEL, 'billing_mode': 'video_multi', 'model_price': 0.2,
     'video_pricing': {'output_per_second': 0.2, 'ref_video_per_second': 0.1, 'resolution_per_second': {'480p': 0.2, '720p': 0.3}}}]}
TLS_SERVER = r'''
const fs=require('fs'),https=require('https');
https.createServer({key:fs.readFileSync('/fixture/key.pem'),cert:fs.readFileSync('/fixture/cert.pem')},(q,r)=>{
  if(q.method!=='GET'||q.url!=='/api/pricing'){r.writeHead(404);r.end();return;}
  if(q.headers.authorization||q.headers.cookie||q.headers['x-api-key']){r.writeHead(400);r.end();return;}
  r.writeHead(200,{'Content-Type':'application/json'});r.end(fs.readFileSync('/fixture/pricing.json'));
}).listen(443,'0.0.0.0');
'''


def request(ip, path, body=None, token=None, method=None, expected=200):
    headers = {'Content-Type': 'application/json', 'Host': 'fixture.kitty.invalid'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request('http://' + ip + ':8080' + path,
        data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(req, timeout=35) as response:
            code, raw = response.status, response.read(4 * 1024 * 1024)
    except urllib.error.HTTPError as error:
        code, raw = error.code, error.read(8192)
    rc.require(code == expected, 'Isolated HTTP check failed: ' + path + ' status=' + str(code))
    return json.loads(raw)


def public_video_model(data, group_id):
    forbidden = {'cost', 'markup', 'applied_markup', 'price_source', 'auto_price', 'follow_increase',
                 'follow_decrease', 'last_synced_at', 'reason', 'api_key', 'base_url'}
    def walk(value):
        if isinstance(value, dict):
            rc.require(not forbidden.intersection(value), 'Private cost/control field leaked into public model DTO')
            for child in value.values():
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)
    walk(data)
    group = next((g for g in data['groups'] if g['id'] == group_id), None)
    rc.require(group is not None and group['platform'] == 'video', 'Priced video group is missing from plaza')
    model = next((m for m in group['models'] if m['name'] == MODEL), None)
    rc.require(model is not None, 'Exact configured video model is missing from plaza')
    pricing = model['pricing']
    rc.require(pricing.get('final_unit_price') is True and pricing['billing_mode'] == 'video', 'Plaza lost final unit-price semantics')
    rc.require(abs(pricing['per_request_price'] - 0.2) < 1e-8 and abs(pricing['reference_per_second'] - 0.1) < 1e-8, 'Public output/reference price mismatch')
    rc.require(pricing['resolution_prices'] == {'480p': 0.2, '720p': 0.3}, 'Public resolution prices changed')
    rc.require(model.get('official_pricing') is None, 'Video DTO contains unrelated official token prices')
    rc.require(model.get('description') and model.get('capabilities', {}).get('supported'), 'Public capability/description missing')
    rc.require(model['capabilities'].get('resolutions') == ['480p'], 'Unexpected fixture capability resolutions')
    return pricing


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    args = parser.parse_args()
    c = rc.contract()
    run_id = 'plaza-v12-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d%H%M%S') + '-' + secrets.token_hex(3)
    root = rc.BASE / 'isolated' / run_id
    root.mkdir(parents=True, mode=0o700)
    network = 'kitty-' + run_id
    label = 'kitty.plaza.isolated=' + run_id
    created = []
    out = {'at': rc.now(), 'run_id': run_id, 'identity': c['identity'], 'passed': False, 'sites': {},
           'external_network': False, 'paid_requests': 0, 'production_database_changes': False,
           'tool_sha256': rc.sha(rc.BASE / 'linux_acceptance.py'), 'common_sha256': rc.sha(rc.BASE / 'release_common.py')}
    def start(name, arguments):
        # Existing names are never adopted. Track before docker run so a failed
        # start can still be inspected; cleanup checks ownership independently.
        rc.require(not rc.run(['docker', 'ps', '-a', '--filter', 'name=^/' + name + '$', '--format', '{{.Names}}']), 'Isolated name collision')
        created.append(name)
        rc.run(['docker', 'run', '-d', '--pull=never', '--name', name, '--label', label, '--network', network, *arguments], timeout=120)
        row = rc.inspect(name)
        rc.require(set(row['NetworkSettings']['Networks']) == {network}, 'Fixture has an unexpected network')
        rc.require(not row['HostConfig'].get('PortBindings'), 'Fixture must not publish ports')
        return row['NetworkSettings']['Networks'][network]['IPAddress']

    def stop(name):
        row = rc.inspect(name)
        rc.require(row['Config'].get('Labels', {}).get('kitty.plaza.isolated') == run_id, 'Refusing to stop a non-fixture container')
        rc.run(['docker', 'stop', '--time', '15', name])
        created.remove(name)

    def sql(pg, body):
        return rc.run(['docker', 'exec', '-i', pg, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'fixture', '-d', 'fixture'], input=body)

    try:
        rc.run(['docker', 'network', 'create', '--internal', '--label', label, network])
        info = json.loads(rc.run(['docker', 'network', 'inspect', network]))[0]
        rc.require(info['Internal'] is True, 'Network is not internal')
        fixture = root / 'supplier'
        fixture.mkdir(mode=0o755)
        rc.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
                '-subj', '/CN=suanliai.top', '-addext', 'subjectAltName=DNS:suanliai.top',
                '-keyout', str(fixture / 'key.pem'), '-out', str(fixture / 'cert.pem')])
        os.chmod(fixture / 'key.pem', 0o600)
        os.chmod(fixture / 'cert.pem', 0o644)
        (fixture / 'pricing.json').write_text(json.dumps(FIXTURE), encoding='utf-8')
        supplier_ip = start(network + '-supplier', ['--user', '0:0', '--read-only', '--memory', '128m', '--cpus', '0.25',
            '-v', str(fixture) + ':/fixture:ro', '--entrypoint', 'node', c['backup']['sites']['kittyapi']['canvas_image'], '-e', TLS_SERVER])
        for site in rc.SITES:
            site_root = root / site
            site_root.mkdir()
            data = site_root / 'data'
            data.mkdir(mode=0o700)
            os.chown(data, 1000, 1000)
            binary = rc.unpack_binary(c['manifest']['sites'][site])
            # Image identities come from the private backup; no live PG query.
            containers = rc.read_json(rc.BASE / 'recovery' / site / 'containers.private.json')
            pgimage = containers[rc.PG[site]]['Image']
            redisname = 'kittyapi-final-c2-20260929t013914z-redis' if site == 'kittyapi' else 'kittyyyds-redis'
            redisimage = containers[redisname]['Image']
            pg, redis = network + '-' + site + '-pg', network + '-' + site + '-redis'
            password, adminpassword = secrets.token_urlsafe(24), secrets.token_urlsafe(24)
            start(pg, ['--memory', '768m', '--cpus', '0.75', '--tmpfs', '/var/lib/postgresql:rw,size=768m',
                '-e', 'POSTGRES_USER=fixture', '-e', 'POSTGRES_DB=fixture', '-e', 'POSTGRES_PASSWORD=' + password, pgimage])
            rc.wait(lambda: rc.run(['docker', 'exec', pg, 'pg_isready', '-h', '127.0.0.1', '-U', 'fixture', '-d', 'fixture']) is not None, 90, 'fixture PG')
            sql(pg, (rc.BASE / 'recovery' / site / 'schema.sql').read_text())
            ledger = c['preflight']['sites'][site]['migrations']
            values = ','.join("('" + name.replace("'", "''") + "','" + value + "')" for name, value in ledger.items())
            sql(pg, 'INSERT INTO schema_migrations(filename,checksum) VALUES ' + values + ';')

            def check_ledger():
                current = json.loads(sql(pg, rc.LEDGER_SQL + ';'))
                rc.require(current == ledger and len(current) == 362, 'Isolated schema/ledger changed')

            check_ledger()
            start(redis, ['--read-only', '--tmpfs', '/data:rw,size=64m', '--memory', '128m', '--cpus', '0.25', redisimage,
                          'redis-server', '--save', '', '--appendonly', 'no'])
            env = {'AUTO_SETUP': 'true', 'DATA_DIR': '/app/data', 'DATABASE_HOST': pg, 'DATABASE_PORT': '5432',
                   'DATABASE_USER': 'fixture', 'DATABASE_PASSWORD': password, 'DATABASE_DBNAME': 'fixture', 'DATABASE_SSLMODE': 'disable',
                   'REDIS_HOST': redis, 'REDIS_PORT': '6379', 'ADMIN_EMAIL': 'fixture@example.invalid', 'ADMIN_PASSWORD': adminpassword,
                   'JWT_SECRET': secrets.token_hex(32), 'SERVER_HOST': '0.0.0.0', 'SERVER_PORT': '8080', 'SERVER_MODE': 'release',
                   'GOMEMLIMIT': '900MiB', 'GOMAXPROCS': '2', 'TZ': 'UTC', 'SSL_CERT_FILE': '/fixture/cert.pem', 'SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS': 'true'}

            def start_app(suffix, executable, managed=False):
                name = network + '-' + site + '-' + suffix
                options = ['--memory', '1536m', '--cpus', '1.5', '--pids-limit', '256', '--read-only', '--user', '1000:1000',
                           '--tmpfs', '/tmp:rw,size=128m,uid=1000,gid=1000', '-v', str(data) + ':/app/data',
                           '-v', str(fixture / 'cert.pem') + ':/fixture/cert.pem:ro', '--add-host', 'suanliai.top:' + supplier_ip,
                           '-v', str(executable) + ':/candidate/sub2api:ro', '--entrypoint', '/candidate/sub2api']
                localenv = dict(env)
                rolefile = None
                if managed:
                    role_dir = site_root / (suffix + '-role')
                    role_dir.mkdir()
                    os.chown(role_dir, 1000, 1000)
                    rolefile = role_dir / 'role'
                    rc.atomic(rolefile, 'standby\n', 0o644, 1000)
                    options += ['-v', str(role_dir) + ':/kitty-role']
                    localenv.update(AUTO_SETUP='false', KITTY_MANAGED_INSTANCE='true', KITTY_INSTANCE_ID=name, KITTY_INSTANCE_ROLE_FILE='/kitty-role/role')
                for key, value in localenv.items():
                    options += ['-e', key + '=' + value]
                ip = start(name, [*options, c['preflight']['sites'][site]['image_id']])
                rc.wait(lambda: request(ip, '/health'), 180, 'fixture app readiness')
                if managed:
                    rc.wait(lambda: rc.ready(rolefile, name, 'standby', False), 90, 'fixture standby')
                return name, ip, rolefile

            previous_binary = rc.BASE / 'recovery' / site / 'previous-sub2api'
            bootstrap, _, _ = start_app('bootstrap', previous_binary)
            stop(bootstrap)  # Synthetic bootstrap only; creates synthetic installation config.
            sql(pg, "UPDATE users SET balance=100 WHERE email='fixture@example.invalid';")
            sql(pg, "INSERT INTO settings(key,value) VALUES ('model_plaza_enabled','true'),('model_plaza_require_auth','false') ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value;")
            check_ledger()
            previous, previous_ip, previous_role = start_app('previous-v11', previous_binary, managed=True)
            rc.require(request(previous_ip, '/api/v1/settings/public')['data']['version'] == rc.OLD_VERSION, 'Wrong old fixture runtime')
            rc.set_role(previous_role, 'standby', 'active')
            rc.wait(lambda: rc.ready(previous_role, previous, 'active', True), 120, 'old fixture leader')
            app, ip, rolefile = start_app('app', binary, managed=True)
            rc.require(request(ip, '/api/v1/settings/public')['data']['version'] == rc.VERSION, 'Wrong fixture version')
            token = request(ip, '/api/v1/auth/login', {'email': 'fixture@example.invalid', 'password': adminpassword})['data']['access_token']
            sql(pg, "INSERT INTO settings(key,value) SELECT 'admin_compliance_acknowledgement:'||id, '{\"version\":\"v2026.06.10\"}' FROM users WHERE email='fixture@example.invalid' ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value;")
            request(ip, '/api/v1/admin/video-upstream', expected=401)
            cfg = request(ip, '/api/v1/admin/video-upstream', token=token)['data']['settings']
            rc.require(cfg['auto_sync'] is False, 'Fixture automatic price sync must remain disabled')
            empty = request(ip, '/api/v1/admin/groups', {'name': 'Isolated empty prices', 'platform': 'video', 'rate_multiplier': 1,
                'is_exclusive': False, 'subscription_type': 'standard', 'model_pricing': []}, token=token)['data']
            gid = empty['id']
            request(ip, '/api/v1/admin/groups/' + str(gid), {'name': 'Saved empty prices', 'model_pricing': []}, token=token, method='PUT')
            rc.require(not request(ip, '/api/v1/admin/groups/' + str(gid), token=token)['data'].get('model_pricing'), 'Empty prices were not retained')
            plaza_empty = request(ip, '/api/v1/model-plaza')['data']
            rc.require(all(g['id'] != gid for g in plaza_empty['groups']), 'An unpriced group was publicly listed')
            before = sql(pg, rc.PRICING_SQL + ';')
            catalog = request(ip, '/api/v1/admin/video-upstream/probe', {}, token=token)['data']
            rc.require(MODEL in catalog['models'] and catalog['models'][MODEL]['spec']['supported'], 'Local fixture price was not parsed')
            rc.require(sql(pg, rc.PRICING_SQL + ';') == before, 'Read-price probe changed group prices')
            upstream = {'platform': 'video', 'models': [MODEL], 'billing_mode': 'video', 'video': {'price_source': 'upstream', 'auto_price': False}}
            request(ip, '/api/v1/admin/groups/' + str(gid), {'model_pricing': [upstream]}, token=token, method='PUT')
            saved = request(ip, '/api/v1/admin/groups/' + str(gid), token=token)['data']['model_pricing'][0]
            rc.require(saved['video']['price_source'] == 'upstream' and abs(saved['per_request_price'] - 0.2) < 1e-8, 'Empty upstream price did not resolve on the server')
            manual = json.loads(json.dumps(saved))
            manual['per_request_price'] = 0
            manual['video'].update(price_source='manual', auto_price=False)
            request(ip, '/api/v1/admin/groups/' + str(gid), {'model_pricing': [manual]}, token=token, method='PUT')
            rc.require(request(ip, '/api/v1/admin/groups/' + str(gid), token=token)['data']['model_pricing'][0]['per_request_price'] == 0, 'Explicit manual zero was lost')
            # Restore a positive synthetic price before the read-only quote check.
            request(ip, '/api/v1/admin/groups/' + str(gid), {'model_pricing': [upstream]}, token=token, method='PUT')
            before = sql(pg, rc.PRICING_SQL + ';')
            request(ip, '/api/v1/admin/video-upstream/probe', {}, token=token)
            rc.require(sql(pg, rc.PRICING_SQL + ';') == before, 'Probe repriced an existing group')
            account = request(ip, '/api/v1/admin/accounts', {'name': 'Synthetic video account', 'platform': 'video', 'type': 'apikey',
                'credentials': {'base_url': 'https://suanliai.top/v1', 'api_key': secrets.token_urlsafe(24)},
                'concurrency': 1, 'priority': 1, 'rate_multiplier': 0.7, 'group_ids': [gid],
                'upstream_billing_probe_enabled': True}, token=token)['data']
            aid = account['id']
            request(ip, '/api/v1/admin/accounts/' + str(aid), {'name': 'Saved synthetic account', 'upstream_billing_probe_enabled': True}, token=token, method='PUT')
            snapshot = request(ip, '/api/v1/admin/accounts/' + str(aid) + '/upstream-billing-probe', {}, token=token)['data']['snapshot']
            rc.require(snapshot['kind'] == 'video_model_prices' and snapshot['status'] == 'ok' and MODEL in snapshot['video_pricing']['models'], 'Account price probe did not successfully read the local supplier')
            rc.require(sql(pg, rc.PRICING_SQL + ';') == before, 'Account probe changed group prices')
            key = request(ip, '/api/v1/keys', {'name': 'Synthetic quote key', 'group_id': gid}, token=token)['data']['key']
            balance = sql(pg, "SELECT balance FROM users WHERE email='fixture@example.invalid';")
            quote_body = {'model': MODEL, 'seconds': 10, 'resolution': '480p', 'aspect_ratio': '16:9'}
            quote = request(ip, '/v1/videos/quote', quote_body, token=key)['data']
            rc.require(abs(quote['estimated_cost'] - 2) < 1e-8, 'Unexpected synthetic quote')
            plaza_before = sql(pg, rc.PRICING_SQL + ';')
            pricing = public_video_model(request(ip, '/api/v1/model-plaza')['data'], gid)
            # This model supports only 480p. A catalog price for 720p must not
            # grant that capability; validate the price mapping separately.
            for resolution in ['480p']:
                price = pricing['resolution_prices'][resolution]
                q = request(ip, '/v1/videos/quote', {**quote_body, 'resolution': resolution}, token=key)['data']
                rc.require(q.get('final_unit_price') is True and abs(q['unit_price'] - price) < 1e-8, 'DTO and quote final unit prices disagree')
                rc.require(abs(q['reference_per_second'] - pricing['reference_per_second']) < 1e-8, 'DTO and quote reference prices disagree')
                rc.require(abs(q['estimated_cost'] - price * quote_body['seconds']) < 1e-8 and q['reference_cost'] == 0, 'DTO and quote totals disagree')
            request(ip, '/v1/videos/quote', {**quote_body, 'resolution': '720p'}, token=key, expected=400)
            rc.require(sql(pg, rc.PRICING_SQL + ';') == plaza_before, 'Public plaza read changed group prices')
            rc.check_assets(site, {'ip': ip}, public=False)
            for path in ['/model-plaza', '/creative-canvas']:
                rc.require(rc.fetch('http://' + ip + ':8080' + path, site + '.xyz')[0] == 200, 'New frontend entry is unavailable')
            request(ip, '/v1/videos/quote', {'model': 'unpriced', 'seconds': 10}, token=key, expected=400)
            rc.require(sql(pg, "SELECT balance FROM users WHERE email='fixture@example.invalid';") == balance, 'Quote changed balance')
            rc.require(sql(pg, 'SELECT count(*) FROM video_tasks;') == '0', 'Fixture unexpectedly generated a video task')
            check_ledger()
            rc.set_role(rolefile, 'standby', 'active')
            rc.set_role(previous_role, 'active', 'draining')
            rc.wait(lambda: rc.ready(rolefile, app, 'active', True) and rc.status(previous_role, previous)['leadership_released'], 180, 'v11 to v12 fixture handoff')
            rc.require(rc.inspect(previous)['State']['Running'], 'Draining old fixture stopped unexpectedly')
            check_ledger()
            out['sites'][site] = {'version': rc.VERSION, 'schema_count': 362, 'ledger_unchanged': True,
                'empty_group_saved': True, 'upstream_empty_price_saved': True, 'manual_zero_saved': True,
                'account_saved': True, 'account_kind': 'video_model_prices', 'probe_preserved_prices': True,
                'quote_did_not_charge': True, 'managed_handoff': True, 'previous_version': rc.OLD_VERSION,
                'plaza_final_reference_resolution_quote_match': True, 'unsupported_resolution_rejected': True,
                'plaza_no_cost_or_markup': True, 'unpriced_group_hidden': True}
            rc.save(root / 'result.json', out)
            print(json.dumps({'site': site, 'isolated_passed': True}), flush=True)
            for name in list(reversed(created)):
                if name.startswith(network + '-' + site + '-'):
                    stop(name)
        out['passed'] = True
    finally:
        errors = []
        for name in list(reversed(created)):
            try:
                stop(name)
            except Exception:
                errors.append(name)
        if errors:
            out['passed'] = False
            out['cleanup_failed'] = errors
        rc.save(root / 'result.json', out)
        if out['passed']:
            rc.save(rc.BASE / 'linux-acceptance.json', out)
        print(json.dumps({'passed': out['passed'], 'run_id': run_id, 'paid_requests': 0}), flush=True)
    rc.require(out['passed'], 'Isolated acceptance did not complete')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'failed': True, 'error_type': type(error).__name__, 'detail': str(error) if type(error) is RuntimeError else 'Inspect private fixture evidence'}), flush=True)
        raise SystemExit(1) from None
