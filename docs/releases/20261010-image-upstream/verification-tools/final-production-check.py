"""Read-only release verification; never creates supplier credentials or paid jobs."""
import json,re,sys
from pathlib import Path
sys.dont_write_bytecode=True
sys.path.insert(0,'/opt/kitty-recovery-20261009/image-upstream-release-r2')
import release_common as rc
out={'at':rc.now(),'production_paid_test_requests':0,'sites':{}}
for site in rc.SITES:
    state=rc.read_json(rc.BASE/(site+'-v14')/'release.json')
    canvas=rc.read_json(Path('/opt/kitty-recovery-20261009/image-upstream-release/canvas-k3')/site/'state.json')
    rc.require(canvas['phase']=='completed','Canvas acceptance incomplete')
    row=rc.inspect(state['new']);rc.require(row['State']['Running'] and row['RestartCount']==0 and not row['State']['OOMKilled'],'Unhealthy backend')
    live=rc.ready(state['role_file'],state['new'],'active',True);rc.require(live,'Backend leader unhealthy')
    saved=rc.query(site,"SELECT COALESCE(json_object_agg(key,value),'{}') FROM settings WHERE key IN ('kitty_image_runtime_v1','kitty_video_runtime_v1')")
    image=json.loads(saved.get('kitty_image_runtime_v1','{}'))
    rc.require(not image.get('auto_sync',False) and not image.get('auto_import_account_models',False) and image.get('markup',0)==0,'New image defaults unexpectedly changed')
    groups=rc.query(site,"SELECT COALESCE(json_agg(t),'[]') FROM (SELECT id,jsonb_array_length(COALESCE(model_pricing,'[]'::jsonb)) AS models FROM groups WHERE platform='video' AND deleted_at IS NULL ORDER BY id LIMIT 100) t")
    accounts=rc.query(site,"SELECT json_build_object('suanli_image_accounts',count(*)) FROM accounts WHERE deleted_at IS NULL AND platform='openai' AND credentials->>'base_url' LIKE 'https://suanliai.top%'")
    task_counts=rc.query(site,"SELECT COALESCE(json_object_agg(status,n),'{}') FROM (SELECT status,count(*) n FROM image_supplier_tasks GROUP BY status) t")
    public=[]
    for host in [site+'.xyz','cdn.'+site+'.xyz']:
        code,raw,_=rc.fetch('https://'+host+'/api/v1/settings/public')
        rc.require(code==200 and json.loads(raw)['data']['version']==rc.VERSION,'Public backend version mismatch')
        code,raw,headers=rc.fetch('https://'+host+'/canvas/canvas?kitty_version='+rc.VERSION)
        chunks=set(p.decode() for p in re.findall(rb'/(?:canvas/)?_next/static/[^"\s<>]+\.js',raw))
        rc.require(code==200 and chunks==set(canvas['chunks']) and 'no-store' in headers.get('Cache-Control',''),'Public Canvas mismatch')
        public.append(host)
    previous=rc.status(state['old_role_file'],state['old'])
    out['sites'][site]={'version':rc.VERSION,'runtime_sha256':state['binary_sha256'],'canvas_version':canvas['version'],'canvas_image':canvas['image'],'video_groups':groups,'image_settings_saved':bool(image),'image_defaults_verified':True,'video_settings':json.loads(saved.get('kitty_video_runtime_v1','{}')),'image_tasks':task_counts,**accounts,'public_hosts':public,'old_running':rc.inspect(state['old'])['State']['Running'],'old_requests':previous['active_requests'],'old_websockets':previous['active_ws']}
out['passed']=True
rc.save(rc.BASE/'final-production.json',out)
print(json.dumps({'passed':True,'sites':{s:{k:v[k] for k in ['version','canvas_version','video_groups','suanli_image_accounts','image_tasks','old_requests','old_websockets']} for s,v in out['sites'].items()}}))
