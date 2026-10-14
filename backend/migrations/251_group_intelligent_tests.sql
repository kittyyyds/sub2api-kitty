-- Extend opt-in group tests to two independent questions. Existing configs and
-- history remain untouched; only explicit schedule_mode=clock configs execute.
ALTER TABLE kitty_candy_tests
    ADD COLUMN IF NOT EXISTS test_type VARCHAR(16) NOT NULL DEFAULT 'candy';

ALTER TABLE kitty_candy_tests
    DROP CONSTRAINT IF EXISTS kitty_candy_tests_group_id_key;
CREATE UNIQUE INDEX IF NOT EXISTS kitty_candy_tests_group_type_unique
    ON kitty_candy_tests(group_id, test_type);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
        WHERE conrelid='kitty_candy_tests'::regclass AND conname='kitty_candy_tests_test_type_check') THEN
        ALTER TABLE kitty_candy_tests
            ADD CONSTRAINT kitty_candy_tests_test_type_check
            CHECK (test_type IN ('candy', 'pelican'));
    END IF;
END $$;

ALTER TABLE kitty_candy_test_runs
    DROP CONSTRAINT IF EXISTS kitty_candy_test_runs_status_check;
ALTER TABLE kitty_candy_test_runs
    ADD CONSTRAINT kitty_candy_test_runs_status_check
    CHECK (status IN ('queued','running','passed','failed','inconclusive','error','cancelled','completed'));

-- Legacy queued jobs cannot be consumed by the newly enabled worker. Keep all
-- rows for history instead of deleting them, and make no upstream requests.
UPDATE kitty_candy_test_runs
SET status='cancelled', finished_at=NOW(),
    result='{"message":"旧测试队列保持暂停；升级没有补发模型请求"}'::jsonb
WHERE status='queued'
  AND (COALESCE(snapshot->>'schedule_mode','')<>'clock'
       OR COALESCE(snapshot->>'test_type','') NOT IN ('candy','pelican'));
