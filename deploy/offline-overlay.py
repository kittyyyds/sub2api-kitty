#!/usr/bin/env python3
"""Build an OCI overlay archive locally from a verified production image.

No Docker daemon or source compilation is used. The archive replaces the full
resources directory and embedded Linux executable, and retains the runtime base.
Requires Docker's containerd image store with OCI archive import support.
"""
import argparse,datetime,gzip,hashlib,io,json,pathlib,re,subprocess,tarfile

def sha(data):return hashlib.sha256(data).hexdigest()
def packed(obj):return json.dumps(obj,ensure_ascii=False,separators=(',',':')).encode()
def blobpath(digest):return 'blobs/sha256/'+digest.removeprefix('sha256:')
def filehash(path):
    h=hashlib.sha256()
    with open(path,'rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''):h.update(block)
    return h.hexdigest()
def checked(args,cwd=None):
    p=subprocess.run(args,cwd=cwd,capture_output=True,text=True)
    if p.returncode:raise RuntimeError('Command failed: '+args[0])
    return p.stdout.strip()

def main():
    ap=argparse.ArgumentParser()
    for name in ['source','base','base-sha256','base-image-id','base-commit','binary','build-receipt','version','site-id','domain','repository','tag','created','output']:
        ap.add_argument('--'+name,required=True)
    a=ap.parse_args();src=pathlib.Path(a.source).resolve();out=pathlib.Path(a.output).resolve()
    if out.exists():raise RuntimeError('Output already exists')
    if filehash(a.base)!=a.base_sha256:raise RuntimeError('Base archive hash mismatch')
    checked(['sh','deploy/verify-kitty-site.sh',a.site_id,a.domain],src)
    origin=checked(['git','remote','get-url','origin'],src)
    if origin not in ['https://github.com/'+a.repository,'https://github.com/'+a.repository+'.git']:raise RuntimeError('Repository label does not match origin')
    commit=checked(['git','rev-parse','HEAD'],src)
    if not re.fullmatch('[a-f0-9]{40}',commit):raise RuntimeError('Invalid source commit')
    if checked(['git','status','--porcelain'],src):raise RuntimeError('Source tree must be clean')
    receipt=json.loads(pathlib.Path(a.build_receipt).read_text(encoding='utf-8'))
    build=checked(['go','version','-m',str(pathlib.Path(a.binary).resolve())])
    settings={}
    for line in build.splitlines():
        fields=line.strip().split('\t',1)
        if len(fields)==2 and fields[0]=='build':
            key,sep,value=fields[1].partition('=')
            if sep:
                if value.startswith('"') and value.endswith('"'):value=json.loads(value)
                settings[key]=value
    required={'GOOS':'linux','GOARCH':'amd64','CGO_ENABLED':'0','-trimpath':'true','vcs.revision':commit,'vcs.modified':'false'}
    if any(settings.get(k)!=v for k,v in required.items()):raise RuntimeError('Binary build settings mismatch')
    # Static builds may embed the IANA timezone database for minimal runtime
    # images. Permit only this explicit addition to the required frontend tag.
    if settings.get('-tags') not in ('embed','embed,timetzdata'):raise RuntimeError('Binary build tags mismatch')
    # Go intentionally omits -ldflags from build info under -trimpath (issue
    # 52372). Bind the recorded invocation to this exact binary's SHA below;
    # production acceptance must additionally execute the Linux --version probe.
    command=receipt.get('build_command')
    if not isinstance(command,list) or command.count('-ldflags')!=1 or '-trimpath' not in command:raise RuntimeError('Missing exact build invocation receipt')
    index=command.index('-ldflags')
    if index+1>=len(command):raise RuntimeError('Missing build ldflags value')
    recorded_ldflags=command[index+1]
    if settings.get('-ldflags') and settings['-ldflags']!=recorded_ldflags:raise RuntimeError('Build-info and receipt ldflags differ')
    ldflags=dict(re.findall(r'(?:^|\s)-X\s+(main\.(?:Version|Commit|Date|BuildType))=([^\s]+)',recorded_ldflags))
    if ldflags!={'main.Version':a.version,'main.Commit':commit,'main.Date':a.created,'main.BuildType':'release'}:raise RuntimeError('Binary release metadata mismatch')
    binary=pathlib.Path(a.binary).read_bytes()
    if not binary.startswith(b'\x7fELF'):raise RuntimeError('Not a Linux ELF binary')
    resources=src/'backend/resources';dist=src/'backend/internal/web/dist'
    if not (dist/'index.html').is_file():raise RuntimeError('Matching frontend is missing')
    frontend={p.relative_to(dist).as_posix():filehash(p) for p in sorted(dist.rglob('*')) if p.is_file()}
    frontend_sha=sha(packed(frontend))
    for k,v in {'source_commit':commit,'version':a.version,'created':a.created,'binary_sha256':sha(binary),'frontend_before_sha256':frontend_sha,'frontend_after_sha256':frontend_sha}.items():
        if receipt.get(k)!=v:raise RuntimeError('Build receipt mismatch: '+k)
    epoch=int(datetime.datetime.fromisoformat(a.created.replace('Z','+00:00')).timestamp())
    layer_stream=io.BytesIO()
    with tarfile.open(fileobj=layer_stream,mode='w',format=tarfile.PAX_FORMAT) as layer:
        def add(name,data=None,directory=False,mode=0o644):
            t=tarfile.TarInfo(name);t.mtime=epoch;t.uid=t.gid=1000;t.uname=t.gname='sub2api';t.mode=mode
            if directory:t.type=tarfile.DIRTYPE;t.mode=0o755
            else:t.size=len(data)
            layer.addfile(t,None if directory else io.BytesIO(data))
        add('app',directory=True);add('app/resources',directory=True)
        add('app/resources/.wh..wh..opq',b'')
        for path in sorted(resources.rglob('*')):
            if path.is_symlink():raise RuntimeError('Resource symlinks are not allowed')
            name='app/resources/'+path.relative_to(resources).as_posix()
            if path.is_dir():add(name,directory=True)
            elif path.is_file():add(name,path.read_bytes())
        add('app/sub2api',binary,mode=0o755)
    raw_layer=layer_stream.getvalue();new_layer=gzip.compress(raw_layer,compresslevel=6,mtime=0)
    new_layer_sha=sha(new_layer)
    with tarfile.open(a.base,'r:*') as base:
        index=json.load(base.extractfile('index.json'))
        if len(index['manifests'])!=1:raise RuntimeError('Expected one base image manifest')
        desc=index['manifests'][0]
        if desc['digest']!=a.base_image_id:raise RuntimeError('Base image ID mismatch')
        manifest_bytes=base.extractfile(blobpath(desc['digest'])).read()
        if 'sha256:'+sha(manifest_bytes)!=desc['digest']:raise RuntimeError('Base manifest corrupted')
        manifest=json.loads(manifest_bytes)
        cfg_bytes=base.extractfile(blobpath(manifest['config']['digest'])).read()
        if 'sha256:'+sha(cfg_bytes)!=manifest['config']['digest']:raise RuntimeError('Base config corrupted')
        cfg=json.loads(cfg_bytes)
        if cfg['architecture']!='amd64' or cfg['os']!='linux':raise RuntimeError('Unexpected base platform')
        if cfg['config'].get('User','') not in ['', 'root', '0', '0:0']:raise RuntimeError('Expected root runtime entrypoint from Kitty overlay Dockerfile')
        if cfg['config'].get('Labels',{}).get('org.opencontainers.image.revision')!=a.base_commit:raise RuntimeError('Base source commit mismatch')
        layers=manifest['layers']
        if len(layers)!=len(cfg['rootfs']['diff_ids']):raise RuntimeError('Base layer count mismatch')
        blobs={}
        for n,item in enumerate(layers):
            data=base.extractfile(blobpath(item['digest'])).read()
            if len(data)!=item['size'] or 'sha256:'+sha(data)!=item['digest']:raise RuntimeError('Base layer corrupted')
            raw=gzip.decompress(data) if '+gzip' in item['mediaType'] else data
            if 'sha256:'+sha(raw)!=cfg['rootfs']['diff_ids'][n]:raise RuntimeError('Base filesystem hash mismatch')
            blobs[blobpath(item['digest'])]=data
    cfg['created']=a.created
    cfg['rootfs']['diff_ids'].append('sha256:'+sha(raw_layer))
    cfg.setdefault('history',[]).append({'created':a.created,'created_by':'Kitty verified local prebuilt overlay '+commit,'comment':'Complete resources replacement and embedded binary; no production compilation'})
    cfg['config'].setdefault('Labels',{}).update({'org.opencontainers.image.version':a.version,'org.opencontainers.image.revision':commit,'org.opencontainers.image.source':'https://github.com/'+a.repository,'org.opencontainers.image.created':a.created,'io.kitty.site':a.site_id,'io.kitty.binary.sha256':sha(binary),'io.kitty.frontend.sha256':frontend_sha,'io.kitty.base.digest':a.base_image_id})
    cfg_bytes=packed(cfg);cfg_sha=sha(cfg_bytes)
    layers.append({'mediaType':'application/vnd.oci.image.layer.v1.tar+gzip','digest':'sha256:'+new_layer_sha,'size':len(new_layer)})
    manifest={'schemaVersion':2,'mediaType':'application/vnd.oci.image.manifest.v1+json','config':{'mediaType':'application/vnd.oci.image.config.v1+json','digest':'sha256:'+cfg_sha,'size':len(cfg_bytes)},'layers':layers}
    manifest_bytes=packed(manifest);manifest_sha=sha(manifest_bytes)
    index={'schemaVersion':2,'mediaType':'application/vnd.oci.image.index.v1+json','manifests':[{'mediaType':manifest['mediaType'],'digest':'sha256:'+manifest_sha,'size':len(manifest_bytes),'annotations':{'io.containerd.image.name':a.tag,'org.opencontainers.image.ref.name':a.tag}}]}
    blobs[blobpath(cfg_sha)]=cfg_bytes;blobs[blobpath(new_layer_sha)]=new_layer;blobs[blobpath(manifest_sha)]=manifest_bytes
    blobs['index.json']=packed(index);blobs['oci-layout']=packed({'imageLayoutVersion':'1.0.0'})
    out.parent.mkdir(parents=True,exist_ok=True)
    with out.open('xb') as raw:
        with gzip.GzipFile(fileobj=raw,mode='wb',compresslevel=1,mtime=0) as gz:
            with tarfile.open(fileobj=gz,mode='w',format=tarfile.PAX_FORMAT) as archive:
                for name,data in sorted(blobs.items()):
                    t=tarfile.TarInfo(name);t.size=len(data);t.mtime=epoch;t.mode=0o644;archive.addfile(t,io.BytesIO(data))
    report={'site_id':a.site_id,'domain':a.domain,'repository':a.repository,'version':a.version,'source_commit':commit,'created':a.created,'tag':a.tag,'image_manifest_digest':'sha256:'+manifest_sha,'image_config_digest':'sha256:'+cfg_sha,'base_image_id':a.base_image_id,'base_archive_sha256':a.base_sha256,'archive_sha256':filehash(out),'archive_bytes':out.stat().st_size,'binary_sha256':sha(binary),'build_receipt_sha256':filehash(a.build_receipt),'frontend_manifest_sha256':frontend_sha,'frontend_files':len(frontend),'resources':{p.relative_to(resources).as_posix():filehash(p) for p in sorted(resources.rglob('*')) if p.is_file()},'new_layer_sha256':new_layer_sha,'new_layer_diff_id':'sha256:'+sha(raw_layer),'full_resources_replaced':True}
    out.with_suffix(out.suffix+'.manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
    out.with_suffix(out.suffix+'.frontend.json').write_bytes(packed(frontend))
    print(json.dumps(report,ensure_ascii=False))

if __name__=='__main__':main()
