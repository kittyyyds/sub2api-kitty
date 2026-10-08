from pathlib import Path
import json,re
ROOT=Path(__file__).resolve().parent
PREV=Path('D:/AI-Storage/Reviews/easypay-compat-fix-20261008')
m=json.loads((ROOT/'build-manifest.json').read_text());old=json.loads((PREV/'build-manifest.json').read_text())
assets={}
for site in ['kittyapi','kittyyyds']:
 allassets=json.loads((ROOT/(site+'-web-assets.json')).read_text())
 # Verify current monitor chunks and previous entry/monitor/dashboard chunks.
 assets[site]={k:v for k,v in allassets.items() if k.startswith('assets/') and any(x in k for x in ['ChannelMonitorView','DashboardView','index-','vendor-','accountTokenGuard'])}
 s=(PREV/('release-'+site+'.py')).read_text()
 s=s.replace('/opt/kitty-recovery-20261008/easypay-compat','/opt/kitty-recovery-20261008/monitor-path')
 s=s.replace("VERSION='2.9.9-kitty.6';OLD=SITE+'-2-9-9-kitty.5-20261008';NEW=SITE+'-2-9-9-kitty.6-20261008'","VERSION='2.9.9-kitty.7';OLD=SITE+'-2-9-9-kitty.6-20261008';NEW=SITE+'-2-9-9-kitty.7-20261008'")
 s=s.replace('2-9-9-kitty.5-rollback-easypay-20261008','2-9-9-kitty.6-rollback-monitor-path-20261008').replace("=='2.9.9-kitty.5'","=='2.9.9-kitty.6'")
 s=s.replace('database-before-easypay-compat.dump','database-before-monitor-path.dump').replace('Caddyfile.easypay-compat','Caddyfile.monitor-path')
 for key,value in [('EXPECTED_SHA',m['sites'][site]['sha256']),('OLD_SHA',old['sites'][site]['sha256']),('ASSETS',assets[site]),('LEGACY_COUNT',m['sites'][site]['legacy_assets_retained'])]:
  s=re.sub('^'+key+r'=.*$',key+'='+repr(value),s,flags=re.M)
 # Fail closed if a concurrent operation changed the production Caddy config.
 s=s.replace("assert token in before", "assert token in before\n  assert sha(caddy)==json.loads((BASE/'preflight.json').read_text())['caddy_sha256']\n  assert 'kitty-fenjue-test-app:8080' in before")
 s=s.replace("atomic(caddy,candidate)","assert caddy.read_text()==before,'Caddy changed since validation'\n  atomic(caddy,candidate)")
 # Rollback changes this site's target only, retaining any unrelated new routes.
 s=s.replace("cb=(ROOT/'Caddyfile.before').read_text().replace(OLD+':8080',rb+':8080')", "cb=caddy.read_text()\n  assert NEW+':8080' in cb or OLD+':8080' in cb\n  cb=cb.replace(NEW+':8080',rb+':8080').replace(OLD+':8080',rb+':8080')")
 assert 'kitty.5' not in s
 compile(s,site,'exec');(ROOT/('release-'+site+'.py')).write_text(s,encoding='utf-8')
 # Standalone recovery script, prepared before routing traffic.
 prefix=s.split('\ntry:\n',1)[0]
 recovery='''
R=json.loads(RECEIPT.read_text());assert R.get('route_may_have_changed')
old=inspect(OLD)
env=dict(x.split('=',1) for x in old['Config']['Env'] if '=' in x)
rolefile=env['KITTY_INSTANCE_ROLE_FILE'];rolemount=next(m for m in old['Mounts'] if rolefile.startswith(m['Destination'].rstrip('/')+'/'))
oldrole=pathlib.Path(rolemount['Source'])
caddy=pathlib.Path('/opt/kitty-migration/caddy/Caddyfile');routefile=pathlib.Path('/opt/kitty-updater/state/routing.json')
def step(name,detail=None):
 R['steps'].append({'at':now(),'name':name,'detail':detail});atomic(RECEIPT,json.dumps(R,indent=2),0o600);print(json.dumps({'site':SITE,'step':name,'detail':detail}),flush=True)
rollback()
'''
 (ROOT/('rollback-'+site+'.py')).write_text(prefix+recovery,encoding='utf-8')
s=(PREV/'accept_template.py').read_text().replace('/opt/kitty-recovery-20261008/easypay-compat','/opt/kitty-recovery-20261008/monitor-path')
s=s.replace("name=site+'-2-9-9-kitty.6-20261008'","name=site+'-2-9-9-kitty.7-20261008'").replace("old=site+'-2-9-9-kitty.5-20261008'","old=site+'-2-9-9-kitty.6-20261008'").replace("=='2.9.9-kitty.6'","=='2.9.9-kitty.7'")
s=s.replace("assert metadata==before['sites'][site]['metadata'],'settings or migration count changed'", "assert metadata['settings']==before['sites'][site]['metadata']['settings'],'settings changed'\n assert metadata['migration_count']==before['sites'][site]['metadata']['migration_count']+1,'unexpected migration count'")
s=s.replace("item['settings_and_migration_count_unchanged']=True", "item['settings_unchanged']=True\n item['one_additive_migration']=True")
s=s.replace("(BASE/('acceptance-'", "assert 'kitty-fenjue-test-app:8080' in pathlib.Path('/opt/kitty-migration/caddy/Caddyfile').read_text()\nassert fetch('https://cs.kittyyyds.xyz/health')[0]==200\n(BASE/('acceptance-'")
for sites in [['kittyapi'],['kittyapi','kittyyyds']]:
 text='SITES='+repr(sites)+'\nMANIFEST='+repr(m['sites'])+'\nASSETS='+repr(assets)+'\n'+s
 compile(text,'accept','exec');(ROOT/('accept-'+('-'.join(sites))+'.py')).write_text(text,encoding='utf-8')
(ROOT/'collect.py').write_text((PREV/'collect.py').read_text().replace('/opt/kitty-recovery-20261008/easypay-compat','/opt/kitty-recovery-20261008/monitor-path'),encoding='utf-8')
print('Release, acceptance and recovery scripts prepared')
