from pathlib import Path
import hashlib
ROOT=Path(__file__).resolve().parent
migration=ROOT/'kittyapi-tree/backend/migrations/267_kitty_channel_monitor_request_path.sql'
sql=migration.read_bytes().decode().strip();digest=hashlib.sha256(sql.encode()).hexdigest()
fn='''
def prepare_monitor_schema():
 sql="""BEGIN;
SET LOCAL statement_timeout='5s';SET LOCAL lock_timeout='750ms';SET LOCAL idle_in_transaction_session_timeout='12s';
DO $$ BEGIN
 IF NOT pg_try_advisory_xact_lock(694208311321144027) THEN RAISE EXCEPTION 'migration lock busy'; END IF;
 IF EXISTS (SELECT 1 FROM schema_migrations WHERE filename='267_kitty_channel_monitor_request_path.sql' AND checksum<>'DIGEST') THEN RAISE EXCEPTION 'migration checksum conflict'; END IF;
END $$;
MIGRATION
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='channel_monitors' AND column_name='request_path' AND data_type='character varying' AND character_maximum_length=500 AND is_nullable='NO' AND column_default=''''''''::character varying') THEN RAISE EXCEPTION 'monitor path schema mismatch'; END IF;
END $$;
INSERT INTO schema_migrations(filename,checksum) VALUES ('267_kitty_channel_monitor_request_path.sql','DIGEST') ON CONFLICT (filename) DO NOTHING;
COMMIT;
"""
 p=subprocess.run(['docker','exec','-i',PG,'sh','-c','PGPASSWORD="$POSTGRES_PASSWORD" psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],input=sql,text=True,capture_output=True,timeout=15)
 if p.returncode:raise RuntimeError('bounded monitor schema preparation failed: '+p.stderr[-500:])
 step('additive_monitor_schema_prepared',{'migration':'267_kitty_channel_monitor_request_path.sql','checksum':'DIGEST','old_instance_compatible':True})
'''
# Use a structural default check with SQL quote escaping, generated explicitly.
fn=fn.replace("AND column_default='''::character varying'",'') if False else fn
start=fn.index(" AND column_default=");end=fn.index(") THEN RAISE",start)
fn=fn[:start]+fn[end:]
fn=fn.replace('MIGRATION',sql).replace('DIGEST',digest)
for site in ['kittyapi','kittyyyds']:
 p=ROOT/('release-'+site+'.py');s=p.read_text()
 s=s.replace('\ntry:\n',fn+'\ntry:\n',1)
 s=s.replace(" shutil.copytree(oldruntime,RUNTIME);", " prepare_monitor_schema()\n shutil.copytree(oldruntime,RUNTIME);")
 compile(s,site,'exec');p.write_text(s,encoding='utf-8')
 if site=='kittyapi':
  prefix=s.split('\ntry:\n',1)[0]
  tail=s[s.index(" new=inspect(NEW);ip="):]
  head='''
try:
 R=json.loads(RECEIPT.read_text());assert not R.get('route_may_have_changed')
 shutil.copy2(RECEIPT,ROOT/'release-first-attempt.json')
 def step(name,detail=None):
  R['steps'].append({'at':now(),'name':name,'detail':detail});atomic(RECEIPT,json.dumps(R,indent=2),0o600);print(json.dumps({'site':SITE,'step':name,'detail':detail}),flush=True)
 old=inspect(OLD);assert old['State']['Running']
 entry=old['Config']['Entrypoint'][0]
 runtime_mount=next(m for m in old['Mounts'] if entry.startswith(m['Destination'].rstrip('/')+'/'))
 env=dict(x.split('=',1) for x in old['Config']['Env'] if '=' in x)
 rolefile=env['KITTY_INSTANCE_ROLE_FILE'];rolemount=next(m for m in old['Mounts'] if rolefile.startswith(m['Destination'].rstrip('/')+'/'))
 oldrole=pathlib.Path(rolemount['Source']);assert (oldrole/'role').read_text().strip()=='active'
 oldruntime=pathlib.Path(runtime_mount['Source'])
 assert sha(ROOT/'previous-sub2api')==OLD_SHA
 assert sha(RUNTIME/pathlib.Path(entry).relative_to(runtime_mount['Destination']))==EXPECTED_SHA
 assert (ROOT/'database-before-monitor-path.dump').stat().st_size>0
 assert not inspect(NEW)['State']['Running']
 prepare_monitor_schema()
 atomic(ROLE/'role','standby\\n',owner=1000)
 docker_api('POST','/containers/'+NEW+'/start')
 hc=old['HostConfig']
 R['phase']='preparing';step('candidate_restarted_after_schema_preparation')
'''
  resume=prefix+head+tail;compile(resume,'resume','exec');(ROOT/'resume-kittyapi.py').write_text(resume,encoding='utf-8')
print('Managed schema preparation added; exact migration checksum '+digest)
