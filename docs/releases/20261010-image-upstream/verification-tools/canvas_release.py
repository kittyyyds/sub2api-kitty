"""Prepared Canvas replacement; preserve old connections and immutable assets."""
import copy
import json
import re
import sys
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, '/opt/kitty-recovery-20261009/image-upstream-release-r2')
import release_common as rc

ARTIFACTS = Path('/opt/kitty-recovery-20261009/image-upstream-release')
BASE = ARTIFACTS / 'canvas-k3'
ARCHIVE = ARTIFACTS / 'kitty-canvas-v0.8.0-kitty.3-linux-amd64.oci.tar.gz'
IMAGE = 'sha256:ac1bb760dd0799ae589807be7b7c020c863b8a066de2b521997a1b0fb969a9b7'
SHA = '02a6986e9c3dd1521d89924b31cab0a56a310d6868a020c9fbdb49771dd0d3e3'


def root(site):
    rc.require(site in rc.SITES, 'Unknown site')
    return BASE / site


def prepare(site):
    with rc.release_lock():
        contract = rc.contract()
        for s in rc.SITES:
            acceptance = rc.read_json(rc.BASE / ('acceptance-' + s + '.json'))
            rc.require(acceptance['passed'] and acceptance['identity'] == contract['identity'], 'Both backend releases must be accepted first')
        rc.require(not root(site).exists(), 'Canvas candidate already prepared; inspect evidence before continuing')
        rc.require(rc.sha(ARCHIVE) == SHA, 'Canvas archive digest mismatch')
        linux=rc.read_json(rc.BASE/'canvas-linux-acceptance.json')
        rc.require(linux['passed'] and linux['image_id']==IMAGE and linux['archive_sha256']==SHA,'Canvas Linux acceptance missing')
        if not rc.run(['docker','image','ls','--no-trunc','--quiet',IMAGE]):
            rc.run(['docker','load','-i',str(ARCHIVE)],timeout=180)
        oldname = rc.CANVAS[site]
        old = rc.inspect(oldname)
        rc.require(old['State']['Running'] and not old['State']['OOMKilled'], 'Old Canvas unhealthy')
        root(site).mkdir(parents=True, mode=0o700)
        rc.save(root(site)/'previous.private.json',old)
        newname=site+'-canvas-0-8-0-kitty3-20261009'
        state={'site':site,'old':oldname,'new':newname,'phase':'preparing','image':IMAGE,'archive_sha256':SHA,'version':'v0.8.0-kitty.3'}
        rc.save(root(site)/'state.json',state)
        cfg=copy.deepcopy(old);cfg['Image']=IMAGE
        env=dict(v.split('=',1) for v in old['Config']['Env'] if '=' in v)
        binds=[m['Source']+':'+m['Destination']+':'+(m.get('Mode') or ('rw' if m['RW'] else 'ro')) for m in old['Mounts']]
        state['ip']=rc.clone(cfg,newname,env,binds)
        rc.save(root(site)/'state.json',state)
        rc.wait(lambda:rc.fetch('http://'+state['ip']+':3101/canvas/canvas')[0]==200,120,'Canvas candidate')
        rc.run(['docker','exec',newname,'node','-e','require("sharp");console.log("native-ok")'])
        rc.require(rc.fetch('http://'+state['ip']+':3101/canvas/api/account')[0]==404,'Upstream account endpoint exposed')
        code,html,_=rc.fetch('http://'+state['ip']+':3101/canvas/canvas')
        rc.require(code==200,'Canvas document missing')
        rc.run(['docker','exec',newname,'node','-e',"const s=require('fs').readFileSync('server.js','utf8');const c=JSON.parse(s.match(/^const nextConfig = (.*)$/m)[1]);if(c.env.NEXT_PUBLIC_APP_VERSION!=='v0.8.0-kitty.3'||c.basePath!=='/canvas')process.exit(1);console.log('compiled-version-ok')"])
        chunks=list(dict.fromkeys(re.findall(rb'/(?:canvas/)?_next/static/[^"\s<>]+\.js',html)))
        rc.require(len(chunks)>3,'Canvas chunks missing')
        for chunk in chunks:
            rc.require(rc.fetch('http://'+state['ip']+':3101'+chunk.decode())[0]==200,'Canvas chunk unavailable')
        state.update(phase='prepared',chunks=[c.decode() for c in chunks],native_verified=True)
        rc.save(root(site)/'state.json',state)
        print(json.dumps({'site':site,'canvas_prepared':True,'chunks':len(chunks)}))


def activate(site):
    with rc.release_lock():
        state=rc.read_json(root(site)/'state.json')
        rc.require(state['phase']=='prepared','Canvas candidate not prepared')
        if site=='kittyyyds':rc.require(rc.read_json(root('kittyapi')/'state.json')['phase']=='completed','Accept API Canvas first')
        target=rc.MAIN if site=='kittyapi' else rc.BACKEND
        before=target.read_text()
        needle=state['old']+':3101'
        rc.require(before.count(needle)==1,'Canvas route differs')
        after=before.replace(needle,state['new']+':3101')
        rc.atomic(root(site)/'caddy-before.private',before)
        pending=target.parent/('image-canvas-'+site+'.pending')
        rc.atomic(pending,after,0o644)
        if site=='kittyyyds':
            main=target.parent/'image-canvas-yyds-main.pending'
            rc.atomic(main,rc.MAIN.read_text().replace('/etc/caddy/backend-router.caddy','/etc/caddy/'+pending.name),0o644)
        else:main=pending
        rc.run(['docker','exec','kitty-migration-caddy','caddy','validate','--config','/etc/caddy/'+main.name,'--adapter','caddyfile'])
        rc.require(target.read_text()==before,'Canvas route changed concurrently')
        state.update(phase='switching',route=str(target),before_sha256=rc.sha(target))
        rc.save(root(site)/'state.json',state)
        rc.atomic(target,after,0o644)
        rc.run(['docker','exec','kitty-migration-caddy','caddy','reload','--config','/etc/caddy/Caddyfile','--adapter','caddyfile'])
        state.update(phase='live',after_sha256=rc.sha(target))
        rc.save(root(site)/'state.json',state)
        print(json.dumps({'site':site,'canvas_switched':True,'old_retained':True}))


def accept(site):
    state=rc.read_json(root(site)/'state.json')
    rc.require(state['phase']=='live','Canvas not switched')
    rc.require(rc.sha(Path(state['route']))==state['after_sha256'],'Route changed after switch')
    rc.require(rc.inspect(state['new'])['Image']==IMAGE and rc.inspect(state['old'])['State']['Running'],'Canvas instance changed')
    url='https://'+site+'.xyz'
    code,html,headers=rc.fetch(url+'/canvas/canvas?image_release=14')
    rc.require(code==200 and 'no-store' in headers.get('Cache-Control',''),'Public Canvas document or cache policy failed')
    chunks=set(c.decode() for c in re.findall(rb'/(?:canvas/)?_next/static/[^"\s<>]+\.js',html))
    rc.require(chunks==set(state['chunks']),'Public Canvas still serves a different build')
    for chunk in state['chunks']:
        rc.require(rc.fetch(url+chunk)[0]==200,'Public Canvas chunk failed')
    rc.require(json.loads(rc.fetch(url+'/api/v1/settings/public')[1])['data']['version']=='2.9.9-kitty.14','Backend regressed')
    state.update(phase='completed',accepted_at=rc.now(),public_chunks_verified=True)
    rc.save(root(site)/'state.json',state)
    print(json.dumps({'site':site,'canvas_accepted':True,'version':state['version'],'old_retained':True}))
