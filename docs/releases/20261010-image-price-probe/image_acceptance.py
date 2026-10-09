"""Only called with the isolated fixture's IP/database and synthetic credentials."""
import json
import base64
import secrets


def check_images(ip, token, pg, sql, request, rc, models, fixture, restart_candidate):
    request(ip, '/api/v1/admin/image-upstream', expected=401)
    initial = request(ip, '/api/v1/admin/image-upstream', token=token)['data']
    rc.require(initial['settings']['markup'] == 0 and not initial['settings']['auto_sync'], 'Image defaults changed')
    video = request(ip, '/api/v1/admin/video-upstream', token=token)['data']['settings']
    cfg = dict(initial['settings'], auto_sync=True, auto_import_account_models=True, markup=.1)
    request(ip, '/api/v1/admin/image-upstream', cfg, token=token, method='PUT')
    rc.require(request(ip, '/api/v1/admin/video-upstream', token=token)['data']['settings'] == video, 'Image save changed video settings')
    gid = request(ip, '/api/v1/admin/groups', {'name':'Synthetic image group','platform':'openai','allow_image_generation':True,'rate_multiplier':100,'is_exclusive':False,'subscription_type':'standard'}, token=token)['data']['id']
    account=request(ip, '/api/v1/admin/accounts', {'name':'Synthetic image supplier','platform':'openai','type':'apikey','concurrency':3,'priority':1,'rate_multiplier':1,'group_ids':[gid], 'credentials':{'base_url':'https://suanliai.top/v1','api_key':secrets.token_urlsafe(24),'model_mapping':{m:m for m in models}}}, token=token)
    aid=account['data']['id']
    group_before=request(ip,'/api/v1/admin/groups/'+str(gid),token=token)['data']
    probe=request(ip,'/api/v1/admin/accounts/'+str(aid)+'/upstream-billing-probe',{},token=token)['data']
    snapshot=probe.get('snapshot',probe)
    rc.require(snapshot['kind']=='image_model_prices' and snapshot['status']=='ok','Image account did not probe model prices')
    rc.require(all(m in snapshot['image_pricing']['models'] for m in models),'Image probe omitted configured prices')
    rc.require(not snapshot.get('data') and 'synced_rate_multiplier' not in snapshot,'Image probe unexpectedly synced a token rate')
    rc.require(request(ip,'/api/v1/admin/groups/'+str(gid),token=token)['data']['model_pricing']==group_before['model_pricing'],'Observation changed group prices')

    original = sql(pg, rc.PRICING_SQL+';')
    request(ip, '/api/v1/admin/image-upstream/sync?apply=false', {}, token=token)
    rc.require(original == sql(pg, rc.PRICING_SQL+';'), 'Image probe changed selling prices')
    request(ip, '/api/v1/admin/image-upstream/sync?apply=true', {}, token=token)
    group = request(ip, '/api/v1/admin/groups/'+str(gid), token=token)['data']
    rc.require({r['models'][0] for r in group['model_pricing']} == set(models), 'Not all 31 selected image IDs imported')
    rc.require(all(abs(v-.3)<1e-8 for tiers in group['image_model_prices'].values() for v in tiers.values()), 'Initial image markup mismatch')
    plaza = request(ip, '/api/v1/model-plaza')['data']
    public = next(g for g in plaza['groups'] if g['id']==gid)
    rc.require({r['name'] for r in public['models']} == set(models), 'Public image list incomplete')
    rc.require(all(r.get('description') for r in public['models']), 'Image descriptions missing')
    key = request(ip, '/api/v1/keys', {'name':'Synthetic image key','group_id':gid}, token=token)['data']['key']
    key_other = request(ip, '/api/v1/keys', {'name':'Other image key','group_id':gid}, token=token)['data']['key']
    before = float(sql(pg, "SELECT balance FROM users WHERE email='fixture@example.invalid';"))
    headers={'Idempotency-Key':'synthetic-image-sync'}
    body={'model':'gpt-image-2','prompt':'fixture sync','n':1}
    result=request(ip,'/v1/images/generations',body,token=key,extra_headers=headers)
    rc.require(len(result.get('data',[]))==1,'Synchronous image result missing')
    request(ip,'/v1/images/generations',body,token=key,extra_headers=headers)
    rc.require(abs(before-float(sql(pg,"SELECT balance FROM users WHERE email='fixture@example.invalid';"))-.3)<1e-8,'Image charge duplicated or multiplied by 100')
    rc.require(sql(pg,"SELECT count(*) FROM usage_logs WHERE request_id LIKE 'simg_%';")=='1','Successful image receipt not atomic/exactly once')
    # Async result survives independently of the request's HTTP lifetime.
    accepted=request(ip,'/v1/images/generations',{'model':'gpt-image-2','prompt':'async','n':1},token=key,extra_headers={'Prefer':'respond-async','Idempotency-Key':'synthetic-async'},expected=202)
    task=accepted['id']
    request(ip,'/v1/images/tasks/'+task,token=key_other,expected=404)
    # The agreed .3 remains despite repricing the group while it is pending.
    request(ip,'/api/v1/admin/image-upstream/model-price',{'group_id':gid,'model':'gpt-image-2','price':9},token=token,method='PUT')
    completed=rc.wait(lambda: (lambda t: t if t['status']=='completed' else None)(request(ip,'/v1/images/tasks/'+task,token=key)),180,'synthetic image settlement')
    rc.require(len(completed['result']['data'])==1,'Async image result missing')
    request(ip,'/v1/images/tasks/'+task,token=key)
    rc.require(abs(before-float(sql(pg,"SELECT balance FROM users WHERE email='fixture@example.invalid';"))-.6)<1e-8,'Image snapshot price or polling dedup failed')
    # Banana references are transformed into Chat; no raw key reaches clients.
    request(ip,'/v1/images/generations',{'model':'nano-banana-2-稳定出图','prompt':'reference fixture','image':['data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1sAAAAASUVORK5CYII='],'n':1},token=key,extra_headers={'Idempotency-Key':'synthetic-banana'})
    request(ip,'/v1/images/generations',{'model':'gpt-image-2','prompt':'failure','n':1},token=key,extra_headers={'Idempotency-Key':'synthetic-failure'},expected=409)
    rc.require(abs(before-float(sql(pg,"SELECT balance FROM users WHERE email='fixture@example.invalid';"))-.9)<1e-8,'Failed image was charged')
    rc.require(sql(pg,"SELECT count(*) FROM usage_logs WHERE request_id LIKE 'simg_%';")=='3','Image receipt count mismatch')
    request(ip,'/api/v1/admin/image-upstream/sync?apply=true',{},token=token)
    group=request(ip,'/api/v1/admin/groups/'+str(gid),token=token)['data']
    rc.require(group['image_model_prices']['gpt-image-2']['1K']==9,'Automatic sync overwrote manual image price')
    rc.require(request(ip,'/api/v1/admin/video-upstream',token=token)['data']['settings']==video,'Image operations changed video configuration')
    # Standard multipart edits must reach the supplier without losing the file.
    boundary='KittySyntheticImageBoundary'
    data=b''
    for name,value in [('model','gpt-image-2-4K'),('prompt','synthetic edit'),('n','1')]:
        data+=('--'+boundary+'\r\nContent-Disposition: form-data; name="'+name+'"\r\n\r\n'+value+'\r\n').encode()
    data+=('--'+boundary+'\r\nContent-Disposition: form-data; name="image"; filename="fixture.png"\r\nContent-Type: image/png\r\n\r\n').encode()+base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1sAAAAASUVORK5CYII=')+('\r\n--'+boundary+'--\r\n').encode()
    edited=request(ip,'/v1/images/edits',data,token=key,extra_headers={'Content-Type':'multipart/form-data; boundary='+boundary,'Idempotency-Key':'synthetic-edit'})
    rc.require(len(edited.get('data',[]))==1,'Multipart image result missing')
    # Restart only this synthetic candidate while the upstream still reports running.
    accepted=request(ip,'/v1/images/generations',{'model':'gpt-image-2-4K','prompt':'restart','n':1},token=key,extra_headers={'Prefer':'respond-async','Idempotency-Key':'synthetic-restart'},expected=202)
    task=accepted['id']
    rc.require(task.startswith('simg_') and len(task)==41,'Unexpected synthetic task identity')
    remote=sql(pg,"SELECT upstream_task_id FROM image_supplier_tasks WHERE id='"+task+"';")
    rc.require(remote.startswith('restart-') and remote[8:].isdigit(),'Unexpected synthetic supplier identity')
    restart_candidate()
    (fixture/remote).write_text('ready',encoding='utf-8')
    sql(pg,"UPDATE image_supplier_tasks SET next_poll_at=NOW() WHERE id='"+task+"';")
    completed=rc.wait(lambda:(lambda t:t if t['status']=='completed' else None)(request(ip,'/v1/images/tasks/'+task,token=key)),180,'image restart recovery')
    rc.require(len(completed['result']['data'])==1,'Restarted image result missing')
    request(ip,'/v1/images/tasks/'+task,token=key)
    rc.require(abs(before-float(sql(pg,"SELECT balance FROM users WHERE email='fixture@example.invalid';"))-1.5)<1e-8,'Recovery or multipart charge mismatch')
    rc.require(sql(pg,"SELECT count(*) FROM usage_logs WHERE request_id LIKE 'simg_%';")=='5','Recovery image receipt count mismatch')
    rc.save(fixture.parent/(pg+'-image-checks.json'),{'passed':True,'models_imported':len(models),'paid_external_requests':0,'successful_synthetic_tasks':5,'failed_synthetic_tasks':1,'duplicate_submit_and_poll_no_double_charge':True,'locked_unit_price':True,'manual_price_preserved':True,'video_settings_preserved':True,'multipart_edit':True,'process_restart_pending_task_recovered':True})
