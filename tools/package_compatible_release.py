#!/usr/bin/env python3
"""Create a local compatible release package; never upload or publish it."""
import argparse
import gzip
import hashlib
import io
import json
import pathlib
import re
import shlex
import subprocess
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
SITE_SYMBOL = 'github.com/Wei-Shaw/sub2api/internal/service.releaseSiteID'
REPOSITORIES = {'kittyapi': 'kittyyyds/sub2api-kitty', 'kittyyyds': 'kittyyyds/sub2api-kittyyyds'}
CAPABILITIES = ["permanent-recharge-ledger-v1", "final-media-pricing-v1",
    "canvas-media-tasks-v1", "account-request-policy-v1", "kitty-console-v1",
    "user-group-denied-models-v1"]

def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def validate_build_info(info, site_id, version, target_os, target_arch):
    """Inspect Go metadata without executing an untrusted/cross-platform binary."""
    if site_id not in REPOSITORIES:
        raise ValueError('unknown release site')
    if not isinstance(info, dict) or not isinstance(info.get('Main'), dict) or not isinstance(info.get('Settings'), list):
        raise ValueError('binary Go build metadata has an invalid structure')
    if info.get('Path') != 'github.com/Wei-Shaw/sub2api/cmd/server' or info['Main'].get('Path') != 'github.com/Wei-Shaw/sub2api':
        raise ValueError('binary is not the Sub2API server module')
    settings = {}
    for row in info.get('Settings', []):
        if not isinstance(row, dict) or not isinstance(row.get('Key'), str) or not isinstance(row.get('Value'), str) or row['Key'] in settings:
            raise ValueError('binary build settings are invalid or ambiguous')
        settings[row['Key']] = row['Value']
    if settings.get('GOOS') != target_os or settings.get('GOARCH') != target_arch:
        raise ValueError('binary target does not match requested OS and architecture')
    if '-ldflags' not in settings and settings.get('-trimpath') == 'true':
        raise ValueError('Go omits linker metadata with -trimpath; rebuild this release without -trimpath to verify site identity')
    tokens = shlex.split(settings.get('-ldflags', ''))
    bindings = {}
    index = 0
    while index < len(tokens):
        item = tokens[index]
        if item == '-X':
            index += 1
            if index >= len(tokens):
                raise ValueError('incomplete build-time string binding')
            binding = tokens[index]
        elif item.startswith('-X='):
            binding = item[3:]
        else:
            index += 1
            continue
        key, sep, value = binding.partition('=')
        if not sep or key in bindings:
            raise ValueError('invalid or duplicate build-time string binding')
        bindings[key] = value
        index += 1
    # Release packages require an explicit injection, even for the default B
    # site. A matching word somewhere in a binary is not a build identity.
    if bindings.get(SITE_SYMBOL) != site_id:
        raise ValueError('binary site injection is missing or does not match --site-id')
    if bindings.get('main.Version') != version:
        raise ValueError('binary version injection does not match the source VERSION')
    return {'site_id': site_id, 'release_repository': REPOSITORIES[site_id],
            'goos': target_os, 'goarch': target_arch, 'version': version}


def inspect_build_info(go_tool, binary, site_id, version, target_os, target_arch):
    result = subprocess.run([go_tool, 'version', '-m', '-json', str(binary)], capture_output=True, timeout=30)
    if result.returncode != 0 or len(result.stdout) > 2 * 1024 * 1024:
        raise ValueError('cannot read bounded Go build metadata')
    try:
        info = json.loads(result.stdout)
    except ValueError as error:
        raise ValueError('binary Go build metadata is invalid') from error
    return validate_build_info(info, site_id, version, target_os, target_arch)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=pathlib.Path, required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    parser.add_argument('--os', choices=['linux', 'windows', 'darwin'], required=True)
    parser.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
    parser.add_argument('--site-id', choices=sorted(REPOSITORIES), required=True)
    parser.add_argument('--go', default='go', help='Go tool with version -m -json support; reads binaries without executing them')
    args = parser.parse_args()
    version = (ROOT/'backend/cmd/server/VERSION').read_text(encoding='utf-8').strip()
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+-kitty\.[1-9][0-9]*', version):
        parser.error('VERSION must identify a numbered Kitty compatibility build')
    args.binary = args.binary.resolve(strict=True)
    if not args.binary.is_file() or args.binary.stat().st_size > 500 * 1024 * 1024:
        parser.error('binary must be a bounded regular file')
    binary_bytes = args.binary.read_bytes()
    if version.encode() not in binary_bytes:
        parser.error('binary does not contain this build VERSION; rebuild with the matching ldflags')
    if args.os == 'linux' and not binary_bytes.startswith(b'\x7fELF'):
        parser.error('the requested Linux package must contain an ELF binary')
    if args.os == 'windows' and not binary_bytes.startswith(b'MZ'):
        parser.error('the requested Windows package must contain a PE binary')
    try:
        build = inspect_build_info(args.go, args.binary, args.site_id, version, args.os, args.arch)
    except (ValueError, OSError, subprocess.TimeoutExpired) as error:
        parser.error(str(error) if isinstance(error, ValueError) else 'Go metadata inspection failed')
    binary_sha = hashlib.sha256(binary_bytes).hexdigest()
    if sha(args.binary) != binary_sha:
        parser.error('binary changed during build identity verification')
    args.output.mkdir(parents=True, exist_ok=True)
    name = f'sub2api_{version}_{args.os}_{args.arch}.tar.gz'
    archive = args.output/name
    if any((args.output / p).exists() for p in (name, 'kitty-compatibility.json', 'checksums.txt')):
        parser.error('release output already exists; choose a new output folder to preserve evidence')
    with archive.open('xb') as raw, gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode='w') as tar:
            info = tar.gettarinfo(str(args.binary), arcname='sub2api')
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            info.mtime = 0
            info.mode = 0o755
            info.size = len(binary_bytes)
            tar.addfile(info, io.BytesIO(binary_bytes))
    contract = {'schema':1, 'family':'kitty-ranxi-console', 'site_id':args.site_id, 'version':version,
        'capabilities':CAPABILITIES,
        'migrations':{p.name:hashlib.sha256(p.read_bytes().strip()).hexdigest()
            for p in sorted((ROOT/'backend/migrations').glob('*.sql'))},
        'artifacts':{name:sha(archive)}}
    descriptor = args.output/'kitty-compatibility.json'
    descriptor.write_text(json.dumps(contract, ensure_ascii=False, sort_keys=True, indent=2)+'\n', encoding='utf-8')
    (args.output/'checksums.txt').write_text(
        f'{sha(archive)}  {name}\n{sha(descriptor)}  {descriptor.name}\n', encoding='utf-8')
    print(json.dumps({'version':version,'archive':str(archive),'archive_sha256':sha(archive),
        'binary_sha256':binary_sha,'active_migrations':len(contract['migrations']), 'build_identity':build,
        'published':False}, ensure_ascii=False))

if __name__ == '__main__':
    main()
