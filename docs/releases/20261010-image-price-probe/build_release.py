"""Build committed .12 source only; never contact a production service.

Run with Python 3.12+. Partial trees are retained and fail closed on reentry.
Completed sites may resume only after their complete artifact hashes match.
"""
from pathlib import Path
import contextlib
import argparse
import datetime
import gzip
import hashlib
import importlib.util
import json
import os
import re
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent
WORK = Path('D:/AI-Storage/Codex/Workspaces/kitty-video-direct-20261009')
OUT = Path('D:\\AI-Storage\\Codex\\Artifacts\\kitty-image-probe-20261010')
PREVIOUS = Path('D:\\AI-Storage\\Codex\\Artifacts\\kitty-image-backend-20261009-r2')
GO = 'D:/AI-Storage/Codex/Workspaces/2026-09-15/bang/work/toolchains/go/bin/go.exe'
PNPM = 'D:/ReverseTools/js/node_modules/.bin/pnpm.cmd'
VERSION = '2.9.9-kitty.15'
OLD_VERSION = '2.9.9-kitty.14'
OLD_COMMIT = '0d68bd04e3d49e8fbebc9d45d4911c38f72309f3'
SITES = ('kittyapi', 'kittyyyds')


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def hashes(path, pattern='*'):
    return {p.relative_to(path).as_posix(): sha(p) for p in sorted(path.rglob(pattern)) if p.is_file()}


def save(path, value):
    temp = path.with_name(path.name + '.tmp')
    with temp.open('w', encoding='utf-8', newline='\n') as stream:
        json.dump(value, stream, indent=2, ensure_ascii=False)
        stream.write('\n')
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temp, path)


def git(*args):
    return subprocess.check_output(['git', *args], cwd=WORK, text=True,
                                   env=dict(os.environ, GIT_OPTIONAL_LOCKS='0')).strip()


@contextlib.contextmanager
def build_lock():
    # Atomic exclusive creation works across Windows processes. Never unlink an
    # existing lock automatically: an interrupted build requires reconciliation.
    path = OUT / 'build.lock'
    with path.open('x', encoding='utf-8') as stream:
        stream.write(str(os.getpid()))
    try:
        yield
    finally:
        path.unlink()


def old_manifest():
    candidates = [PREVIOUS / 'build-manifest.json']
    found = [p for p in candidates if p.is_file()]
    require(bool(found), 'Missing .11 build manifest')
    result = json.loads(found[0].read_text(encoding='utf-8-sig'))
    require(result['version'] == OLD_VERSION and result['commit'] == OLD_COMMIT, 'Unexpected .11 identity')
    for path in found[1:]:
        other = json.loads(path.read_text(encoding='utf-8-sig'))
        require(other['version'] == result['version'] and other['commit'] == result['commit'], '.11 manifests disagree')
        require(all(other['sites'][s]['sha256'] == result['sites'][s]['sha256'] for s in SITES), '.11 binary hashes disagree')
    return result


def build():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--commit', required=True, help='Exact reviewed .12 freeze commit; no implicit HEAD')
    args = parser.parse_args()
    require(re.fullmatch(r'[0-9a-f]{40}', args.commit), 'Supply the full frozen commit SHA')
    commit = git('rev-parse', 'HEAD')
    require(commit == args.commit, 'HEAD differs from the reviewed freeze commit')
    require(not git('status', '--porcelain'), 'Commit all source changes before building')
    require(git('show', 'HEAD:backend/cmd/server/VERSION') == VERSION, 'Committed HEAD VERSION must be .12')
    require(git('diff', '--name-only', OLD_COMMIT, commit, '--', 'backend/migrations/*.sql') == '', 'Only the reviewed additive image task schema may change')
    require(not git('diff', '--name-only', OLD_COMMIT, commit, '--', 'frontend/pnpm-lock.yaml', 'frontend/package.json'), 'Dependency changes require a separate locked dependency install; do not reuse the old node_modules')
    old = old_manifest()
    preflight_path = ROOT / 'server-preflight.json'
    require(preflight_path.is_file(), 'Download the new base/preflight.json as ROOT/server-preflight.json first')
    preflight = json.loads(preflight_path.read_text(encoding='utf-8-sig'))
    preflight_sha = sha(preflight_path)
    require(preflight.get('release_id') == 'image-price-probe-20261010' and preflight.get('target_version') == VERSION, 'This is not a new plaza preflight')
    require(preflight.get('previous_manifest_sha256') == sha(PREVIOUS / 'build-manifest.json'), 'Preflight and .11 build manifest differ')
    for site in SITES:
        baseline = preflight['sites'][site]
        require(baseline['name'] == site + '-2-9-9-kitty.14-20261009', 'Unexpected baseline instance')
        require(baseline['running'] and baseline['binary_sha256'] == old['sites'][site]['sha256'], 'Live baseline does not match .11 manifest')
        require(len(baseline['migrations']) == 363, 'Fresh preflight must contain all 363 migrations')
        require(sha(Path(old['sites'][site]['binary'])) == baseline['binary_sha256'], '.11 binary does not match live baseline')
        previous_sql = PREVIOUS / (site + '-tree') / 'backend/migrations'
        require(hashes(previous_sql, '*.sql') == old['sites'][site]['migration_files_sha256'], 'Prior SQL bytes disagree with .11 manifest')
    OUT.mkdir(parents=True, exist_ok=True)
    with build_lock():
        manifest_path = OUT / 'build-manifest.json'
        manifest = {'version': VERSION, 'commit': commit, 'build_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    'previous_version': OLD_VERSION, 'previous_commit': OLD_COMMIT, 'schema_count': 363,
                    'added_migrations': [], 'canvas_changed': False, 'release_id': 'image-price-probe-20261010', 'previous_manifest_sha256': sha(PREVIOUS / 'build-manifest.json'), 'preflight_sha256': preflight_sha, 'sites': {}}
        if manifest_path.exists():
            manifest = json.loads(manifest_path.read_text(encoding='utf-8-sig'))
            require(manifest['version'] == VERSION and manifest['commit'] == commit, 'OUT belongs to another source commit')
            require(manifest['preflight_sha256'] == preflight_sha, 'Preflight changed since this build began')
        else:
            require(not any(OUT.iterdir()) or set(p.name for p in OUT.iterdir()) == {'build.lock'}, 'OUT is not a fresh release directory')
            save(manifest_path, manifest)
        shutil.copyfile(PREVIOUS / 'build-manifest.json', OUT / 'previous-build-manifest.json')
        shutil.copyfile(preflight_path, OUT / 'build-preflight.json')
        archive_path = OUT / 'source.tar'
        if archive_path.exists():
            require('source_sha256' in manifest, 'Interrupted source archive; reconcile partial OUT before retry')
        else:
            with archive_path.open('xb') as stream:
                subprocess.run(['git', 'archive', commit], cwd=WORK, stdout=stream, check=True)
        source_sha = sha(archive_path)
        require(manifest.get('source_sha256', source_sha) == source_sha, 'Source archive changed')
        manifest['source_sha256'] = source_sha
        save(manifest_path, manifest)
        for site in SITES:
            if site in manifest['sites']:
                entry = manifest['sites'][site]
                for name, digest in entry['artifact_hashes'].items():
                    require(sha(OUT / name) == digest, 'Completed artifact changed: ' + name)
                continue
            previous = PREVIOUS / (site + '-tree')
            previous_binary = Path(old['sites'][site]['binary'])
            require(sha(previous_binary) == old['sites'][site]['sha256'], '.11 binary mismatch: ' + site)
            old_dist = previous / 'backend/internal/web/dist'
            previous_assets = json.loads((PREVIOUS / (site + '-web-assets.json')).read_text(encoding='utf-8-sig'))
            require(sha(PREVIOUS / (site + '-web-assets.json')) == old['sites'][site]['artifact_hashes'][site + '-web-assets.json'], 'Previous asset manifest changed')
            require(hashes(old_dist) == previous_assets, '.11 frontend tree changed: ' + site)
            tree = OUT / (site + '-tree')
            tree.mkdir(exist_ok=False)
            with tarfile.open(archive_path) as archive:
                archive.extractall(tree, filter='data')
            migrations = tree / 'backend/migrations'
            old_sql = hashes(previous / 'backend/migrations', '*.sql')
            require(set(hashes(migrations, '*.sql')) == set(old_sql), 'Unexpected SQL file set')
            for name in old_sql:
                shutil.copy2(previous / 'backend/migrations' / name, migrations / name)
            require(all(hashes(migrations, '*.sql')[n]==v for n,v in old_sql.items()), 'Historical SQL bytes changed')
            ledger = dict(preflight['sites'][site]['migrations'])
            require(len(ledger) == 363, 'Expected fresh schema363 ledger')
            historical = dict(re.findall(r'"([^"\n]+\.sql)":\s*"([a-f0-9]{64})"', (tree / 'backend/internal/repository/managed_schema.go').read_text(encoding='utf-8-sig')))
            for name, digest in ledger.items():
                path = migrations / name
                actual = hashlib.sha256(path.read_bytes().strip()).hexdigest() if path.exists() else historical.get(name)
                require(actual == digest, 'Migration ledger mismatch: ' + name)
            save(OUT / (site + '-migration-ledger.json'), ledger)
            # Build inside the committed snapshot, retaining the caller's worktree.
            modules = WORK / 'frontend/node_modules'
            require(modules.is_dir(), 'Install frontend dependencies in the source worktree first')
            target = tree / 'frontend/node_modules'
            if os.name == 'nt':
                subprocess.run(['cmd', '/d', '/c', 'mklink', '/J', str(target), str(modules)], check=True, stdout=subprocess.DEVNULL)
            else:
                target.symlink_to(modules, target_is_directory=True)
            env = dict(os.environ, VITE_KITTY_SITE=site, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0', GOPROXY='https://goproxy.cn,direct')
            with (OUT / (site + '-frontend.log')).open('w', encoding='utf-8') as log:
                subprocess.run([PNPM, 'build'], cwd=tree / 'frontend', env=env, stdout=log, stderr=log, check=True)
            dist = tree / 'backend/internal/web/dist'
            require((dist / 'index.html').is_file(), 'Missing frontend build')
            new_assets = hashes(dist)
            retained = []
            for path in sorted((old_dist / 'assets').rglob('*')):
                if not path.is_file():
                    continue
                relative = path.relative_to(old_dist)
                dest = dist / relative
                if dest.exists():
                    require(sha(dest) == sha(path), 'Immutable static resource collision: ' + relative.as_posix())
                else:
                    dest.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(path, dest)
                retained.append(relative.as_posix())
            assets = hashes(dist)
            save(OUT / (site + '-web-assets.json'), assets)
            save(OUT / (site + '-new-web-assets.json'), new_assets)
            save(OUT / (site + '-legacy-assets.json'), retained)
            binary = OUT / f'sub2api-{site}-{VERSION}-linux-amd64'
            flags = f'-s -w -buildid= -X main.Version={VERSION} -X main.Commit={commit} -X main.Date={manifest["build_utc"]} -X main.BuildType=release -X github.com/Wei-Shaw/sub2api/internal/service.releaseSiteID={site}'
            with (OUT / (site + '-backend.log')).open('w', encoding='utf-8') as log:
                subprocess.run([GO, 'build', '-mod=readonly', '-p', '2', '-tags', 'embed', '-buildvcs=false', '-ldflags', flags, '-o', str(binary), './cmd/server'], cwd=tree / 'backend', env=env, stdout=log, stderr=log, check=True)
            spec = importlib.util.spec_from_file_location('package_release', tree / 'tools/package_compatible_release.py')
            package = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(package)
            identity = package.inspect_build_info(GO, binary, site, VERSION, 'linux', 'amd64')
            with binary.open('rb') as src, gzip.open(str(binary) + '.gz', 'wb', compresslevel=5) as dest:
                shutil.copyfileobj(src, dest)
            filenames = [binary.name, binary.name + '.gz', *(site + suffix for suffix in ['-web-assets.json', '-new-web-assets.json', '-legacy-assets.json', '-migration-ledger.json'])]
            manifest['sites'][site] = {'binary': str(binary), 'sha256': sha(binary), 'compressed_sha256': sha(Path(str(binary) + '.gz')),
                'bytes': binary.stat().st_size, 'ui_index_sha256': sha(dist / 'index.html'), 'identity': identity,
                'legacy_assets_retained': len(retained), 'historical_migrations_unchanged': True, 'added_migrations': [],
                'previous_sha256': old['sites'][site]['sha256'], 'migration_files_sha256': hashes(migrations,'*.sql'),
                'artifact_hashes': {name: sha(OUT / name) for name in filenames}}
            save(manifest_path, manifest)
            print(json.dumps({'site': site, 'built': True, 'version': VERSION, 'commit': commit}), flush=True)
        manifest['completed_at'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        save(manifest_path, manifest)
        print(json.dumps({'complete': True, 'manifest': str(manifest_path)}), flush=True)


if __name__ == '__main__':
    build()
