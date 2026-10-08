from pathlib import Path
import subprocess,os,json,hashlib,tarfile,io,shutil,datetime,gzip
ROOT=Path(__file__).resolve().parent
WORK=Path('D:/AI-Storage/Codex/Workspaces/kitty-monitor-path-20261008')
PREVIOUS=Path('D:/AI-Storage/Reviews/easypay-compat-fix-20261008')
GO=Path('D:/AI-Storage/Codex/Workspaces/2026-09-15/bang/work/toolchains/go/bin/go.exe')
PNPM=Path('D:/ReverseTools/js/node_modules/.bin/pnpm.cmd')
VERSION='2.9.9-kitty.7'
commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=WORK,text=True).strip()
assert not subprocess.check_output(['git','status','--porcelain'],cwd=WORK,text=True).strip()
raw=subprocess.check_output(['git','archive',commit],cwd=WORK);(ROOT/'source.tar').write_bytes(raw)
manifest={'version':VERSION,'commit':commit,'build_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'sites':{}}
if (ROOT/'build-manifest.json').exists():
 manifest=json.loads((ROOT/'build-manifest.json').read_text());assert manifest['commit']==commit
def sha(p):
 with p.open('rb') as f:return hashlib.file_digest(f,'sha256').hexdigest()
def hashes(base):return {p.relative_to(base).as_posix():sha(p) for p in base.rglob('*') if p.is_file()}
for site in ['kittyapi','kittyyyds']:
 if site in manifest['sites']:
  assert sha(Path(manifest['sites'][site]['binary']))==manifest['sites'][site]['sha256'];continue
 env=dict(os.environ,VITE_KITTY_SITE=site)
 with (ROOT/(site+'-frontend.log')).open('w',encoding='utf-8') as log:
  subprocess.run([str(PNPM),'build'],cwd=WORK/'frontend',env=env,stdout=log,stderr=log,check=True)
 print(json.dumps({'frontend_built':site}),flush=True)
 tree=ROOT/(site+'-tree');tree.mkdir(exist_ok=True)
 with tarfile.open(fileobj=io.BytesIO(raw)) as archive:archive.extractall(tree,filter='data')
 previous=PREVIOUS/(site+'-tree')
 # Production's already-applied migration bytes are immutable.
 for p in (previous/'backend/migrations').iterdir():
  if p.is_file():shutil.copy2(p,tree/'backend/migrations'/p.name)
 oldm=hashes(previous/'backend/migrations');newm=hashes(tree/'backend/migrations')
 assert all(newm[k]==v for k,v in oldm.items())
 assert set(newm)-set(oldm)=={'267_kitty_channel_monitor_request_path.sql'}
 dist=tree/'backend/internal/web/dist'
 shutil.copytree(WORK/'backend/internal/web/dist',dist,dirs_exist_ok=True)
 js=''.join(p.read_text(encoding='utf-8') for p in dist.rglob('*.js'))
 assert 'request_path' in js and '自定义接口路径' in js
 assert 'home-welcome-card' in js and 'monitor-compact-grid' in js
 assert 'local-preview-not-a-credential' not in js and 'fixtureAdapter' not in js
 assert ('https://'+site+'.xyz') in js
 assert ('oRR8GUdEPK' if site=='kittyapi' else 'Wr3pNr9zMe') in js
 legacy=[]
 olddist=previous/'backend/internal/web/dist'
 for p in (olddist/'assets').rglob('*'):
  if not p.is_file():continue
  rel=p.relative_to(olddist);dst=dist/rel
  if dst.exists():assert sha(dst)==sha(p),'immutable asset collision'
  else:shutil.copy2(p,dst);legacy.append(rel.as_posix())
 assets=hashes(dist)
 (ROOT/(site+'-web-assets.json')).write_text(json.dumps(assets,indent=2))
 (ROOT/(site+'-legacy-assets.json')).write_text(json.dumps(legacy,indent=2))
 binary=ROOT/f'sub2api-{site}-{VERSION}'
 flags=f'-s -w -buildid= -X main.Version={VERSION} -X main.Commit={commit} -X main.Date={manifest["build_utc"]} -X main.BuildType=release -X github.com/Wei-Shaw/sub2api/internal/service.releaseSiteID={site}'
 env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
 with (ROOT/(site+'-build.log')).open('w',encoding='utf-8') as log:
  subprocess.run([str(GO),'build','-mod=readonly','-p','2','-tags','embed','-buildvcs=false','-ldflags',flags,'-o',str(binary),'./cmd/server'],cwd=tree/'backend',env=env,stdout=log,stderr=log,check=True)
 with binary.open('rb') as src,gzip.open(str(binary)+'.gz','wb',compresslevel=5) as dst:shutil.copyfileobj(src,dst)
 manifest['sites'][site]={'binary':str(binary),'sha256':sha(binary),'bytes':binary.stat().st_size,'ui_index_sha256':sha(dist/'index.html'),'legacy_assets_retained':len(legacy),'web_asset_files':len(assets),'historical_migrations_unchanged':True,'added_migration':'267_kitty_channel_monitor_request_path.sql','site_identity_verified':True}
 (ROOT/'build-manifest.json').write_text(json.dumps(manifest,indent=2))
 print(json.dumps({'built':site,**manifest['sites'][site]}),flush=True)
