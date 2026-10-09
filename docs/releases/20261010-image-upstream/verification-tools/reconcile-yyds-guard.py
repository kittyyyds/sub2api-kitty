"""Read-only proof that exactly one expected automatic supplier increase explains drift."""
import json,sys,urllib.request
sys.dont_write_bytecode=True
sys.path.insert(0,'/opt/kitty-recovery-20261009/image-upstream-release-r2')
import release_common as rc
model='全能sd2.0(满血原生过脸-933-480p)'
c=rc.contract();old=c['preflight']['sites']['kittyyyds']
original=rc.PRICE_FIELDS_SQL
replacement=original.replace("(p->'video')-'synced_at'","(CASE WHEN g.id=35 AND p->'models'=jsonb_build_array('"+model+"'::text) THEN jsonb_set(p->'video','{cost,unit_price}','2.85'::jsonb) ELSE p->'video' END)-'synced_at'")
replacement=replacement.replace("jsonb_set(p,'{video}',", "jsonb_set(CASE WHEN g.id=35 AND p->'models'=jsonb_build_array('"+model+"'::text) THEN jsonb_set(p,'{per_request_price}','3.35'::jsonb) ELSE p END,'{video}',")
with rc.release_lock():
    rc.require(not rc.state_root('kittyyyds').exists(),'YYDS mutation already began')
    current=rc.query('kittyyyds',rc.PRICING_SQL);protected=rc.protected('kittyyyds')
    rc.require(rc.query('kittyyyds',rc.PRICING_SQL.replace(original,replacement))==old['video_price_digest'],'Other video prices changed')
    rc.require(rc.query('kittyyyds',rc.PROTECTED_SQL['groups_sha256'].replace(original,replacement))==old['protected']['groups_sha256'],'Other group fields changed')
    for key in ['plugins_sha256','settings_sha256']:rc.require(protected[key]==old['protected'][key],'Protected setting changed')
    row=rc.query('kittyyyds',"SELECT p FROM groups g CROSS JOIN LATERAL jsonb_array_elements(g.model_pricing) p WHERE g.id=35 AND p->'models'=jsonb_build_array('"+model+"'::text)")
    rc.require(row['per_request_price']==3.7 and row['video']['cost']['unit_price']==3.2 and row['video']['applied_markup']['per_request']==.5 and row['video']['auto_price'],'Unexpected automatic price')
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open('https://suanliai.top/api/pricing',timeout=20) as response:catalog=json.load(response)
    price=next(p for p in catalog['data'] if p['model_name']==model)
    rc.require(price['model_price']==3.2 and price['billing_mode']=='per_request','Public supplier price does not confirm increase')
    amendment={'at':rc.now(),'identity':c['identity'],'site':'kittyyyds','original_video_price_digest':old['video_price_digest'],'video_price_digest':current,'protected':protected,'only_change':{'group':35,'model':model,'old_cost':2.85,'new_cost':3.2,'old_price':3.35,'new_price':3.7,'markup':.5},'reverse_normalization_matches_original_all_groups':True,'public_supplier_verified':True,'production_writes':False}
    target=rc.BASE/'yyds-price-amendment.json';rc.require(not target.exists(),'Amendment already exists')
    rc.save(target,amendment)
    print(json.dumps({'verified':True,'sha256':rc.sha(target),'production_writes':False}))
