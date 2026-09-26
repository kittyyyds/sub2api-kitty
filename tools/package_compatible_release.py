#!/usr/bin/env python3
"""Create a local compatible release package; never upload or publish it."""
import argparse
import gzip
import hashlib
import json
import pathlib
import re
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
CAPABILITIES = ["permanent-recharge-ledger-v1", "final-media-pricing-v1",
    "canvas-media-tasks-v1", "account-request-policy-v1", "kitty-console-v1",
    "user-group-denied-models-v1"]

def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=pathlib.Path, required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    parser.add_argument('--os', choices=['linux', 'windows', 'darwin'], required=True)
    parser.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
    args = parser.parse_args()
    version = (ROOT/'backend/cmd/server/VERSION').read_text(encoding='utf-8').strip()
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+-kitty\.[1-9][0-9]*', version):
        parser.error('VERSION must identify a numbered Kitty compatibility build')
    args.binary = args.binary.resolve(strict=True)
    binary_bytes = args.binary.read_bytes()
    if version.encode() not in binary_bytes:
        parser.error('binary does not contain this build VERSION; rebuild with the matching ldflags')
    if args.os == 'linux' and not binary_bytes.startswith(b'\x7fELF'):
        parser.error('the requested Linux package must contain an ELF binary')
    if args.os == 'windows' and not binary_bytes.startswith(b'MZ'):
        parser.error('the requested Windows package must contain a PE binary')
    args.output.mkdir(parents=True, exist_ok=True)
    name = f'sub2api_{version}_{args.os}_{args.arch}.tar.gz'
    archive = args.output/name
    if archive.exists():
        parser.error('output archive already exists; choose a new output folder to preserve evidence')
    with archive.open('wb') as raw, gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode='w') as tar:
            info = tar.gettarinfo(str(args.binary), arcname='sub2api')
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            info.mtime = 0
            info.mode = 0o755
            with args.binary.open('rb') as binary:
                tar.addfile(info, binary)
    contract = {'schema':1, 'family':'kitty-ranxi-console', 'version':version,
        'capabilities':CAPABILITIES,
        'migrations':{p.name:hashlib.sha256(p.read_bytes().strip()).hexdigest()
            for p in sorted((ROOT/'backend/migrations').glob('*.sql'))},
        'artifacts':{name:sha(archive)}}
    descriptor = args.output/'kitty-compatibility.json'
    descriptor.write_text(json.dumps(contract, ensure_ascii=False, sort_keys=True, indent=2)+'\n', encoding='utf-8')
    (args.output/'checksums.txt').write_text(
        f'{sha(archive)}  {name}\n{sha(descriptor)}  {descriptor.name}\n', encoding='utf-8')
    print(json.dumps({'version':version,'archive':str(archive),'archive_sha256':sha(archive),
        'binary_sha256':sha(args.binary),'active_migrations':len(contract['migrations']),
        'published':False}, ensure_ascii=False))

if __name__ == '__main__':
    main()
