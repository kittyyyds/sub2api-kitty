"""Run packaged Canvas without production environment, mounts or network."""
import json,re,secrets,sys
from pathlib import Path
sys.dont_write_bytecode=True
sys.path.insert(0,'/opt/kitty-recovery-20261009/image-upstream-release-r2')
import release_common as rc
base=Path('/opt/kitty-recovery-20261009/image-upstream-release')
archive=base/'kitty-canvas-v0.8.0-kitty.3-linux-amd64.oci.tar.gz'
rc.require(rc.sha(archive)=='02a6986e9c3dd1521d89924b31cab0a56a310d6868a020c9fbdb49771dd0d3e3','Canvas archive changed')
rc.run(['docker','load','-i',str(archive)],timeout=180)
info=json.loads(rc.run(['docker','image','inspect','sha256:ac1bb760dd0799ae589807be7b7c020c863b8a066de2b521997a1b0fb969a9b7']))[0]
image=info['Id'];labels=info['Config']['Labels']
rc.require(labels['org.opencontainers.image.revision']=='cb5f6418a5fc7447182363c87dae6d2bcc1a10c1','Canvas source mismatch')
name='kitty-image-canvas-isolated-'+secrets.token_hex(6)
rc.run(['docker','run','-d','--pull=never','--name',name,'--network','none','--label','kitty.image.isolated='+name,'--memory','768m','--cpus','0.5','--pids-limit','128','--read-only','--tmpfs','/tmp:rw,size=64m,uid=1000,gid=1000',image])
try:
    def fetch(path):
        return json.loads(rc.run(['docker','exec',name,'node','-e',"fetch('http://127.0.0.1:3101'+process.argv[1]).then(async r=>console.log(JSON.stringify({status:r.status,body:await r.text(),cache:r.headers.get('cache-control')})))",path],timeout=30))
    rc.wait(lambda:fetch('/canvas/canvas')['status']==200,120,'Isolated Canvas readiness')
    html=fetch('/canvas/canvas')
    rc.run(['docker','exec',name,'node','-e',"const s=require('fs').readFileSync('server.js','utf8');const c=JSON.parse(s.match(/^const nextConfig = (.*)$/m)[1]);if(c.env.NEXT_PUBLIC_APP_VERSION!=='v0.8.0-kitty.3'||c.basePath!=='/canvas')process.exit(1);console.log('compiled-version-ok')"])
    chunks=list(dict.fromkeys(re.findall(r'/(?:canvas/)?_next/static/[^"\s<>]+\.js',html['body'])))
    rc.require(len(chunks)>3,'Canvas static chunks missing')
    for p in chunks:rc.require(fetch(p)['status']==200,'Canvas chunk failed')
    rc.require(fetch('/canvas/api/account')['status']==404,'Unexpected original account API')
    rc.run(['docker','exec',name,'node','-e','require("sharp");console.log("native-ok")'])
    receipt={'passed':True,'image_id':image,'archive_sha256':rc.sha(archive),'chunks':chunks,'source_commit':labels['org.opencontainers.image.revision'],'external_network':False,'production_changes':False,'native_sharp':True,'at':rc.now()}
    rc.save(rc.BASE/'canvas-linux-acceptance.json',receipt)
    print(json.dumps({k:receipt[k] for k in ['passed','image_id','native_sharp']}))
finally:
    row=rc.inspect(name)
    rc.require(row['Config']['Labels'].get('kitty.image.isolated')==name,'Unexpected fixture ownership')
    rc.run(['docker','stop','--time','10',name])
