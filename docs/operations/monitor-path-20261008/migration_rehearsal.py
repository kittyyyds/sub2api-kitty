import json,subprocess
pg='kittyapi-final-c2-20260929t013914z-postgres'
sql="""BEGIN;
SET LOCAL statement_timeout='5s';SET LOCAL lock_timeout='750ms';SET LOCAL idle_in_transaction_session_timeout='12s';
CREATE TEMP TABLE monitor_path_rehearsal (id bigint PRIMARY KEY, name text NOT NULL);
INSERT INTO monitor_path_rehearsal VALUES (1,'existing monitor');
ALTER TABLE monitor_path_rehearsal ADD COLUMN IF NOT EXISTS request_path VARCHAR(500) NOT NULL DEFAULT '';
ALTER TABLE monitor_path_rehearsal ADD COLUMN IF NOT EXISTS request_path VARCHAR(500) NOT NULL DEFAULT '';
INSERT INTO monitor_path_rehearsal(id,name) VALUES (2,'old instance insert');
UPDATE monitor_path_rehearsal SET request_path='/v1/chat/completions' WHERE id=1;
DO $$ BEGIN
 IF (SELECT request_path FROM monitor_path_rehearsal WHERE id=2)<>'' THEN RAISE EXCEPTION 'default regression'; END IF;
 IF (SELECT request_path FROM monitor_path_rehearsal WHERE id=1)<>'/v1/chat/completions' THEN RAISE EXCEPTION 'write regression'; END IF;
END $$;
SELECT json_build_object('existing_row_compatible',true,'old_insert_compatible',true,'repeat_safe',true,'custom_path_persisted',true);
ROLLBACK;
"""
p=subprocess.run(['docker','exec','-i',pg,'sh','-c','PGPASSWORD="$POSTGRES_PASSWORD" psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],input=sql,text=True,capture_output=True,timeout=20)
assert p.returncode==0,'temporary-table migration rehearsal failed'
print(p.stdout.strip())
