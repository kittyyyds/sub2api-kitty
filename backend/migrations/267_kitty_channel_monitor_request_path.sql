-- Additive: old instances keep working during traffic handover and rollback.
SET LOCAL lock_timeout = '750ms';
SET LOCAL statement_timeout = '5s';
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS request_path VARCHAR(500) NOT NULL DEFAULT '';
