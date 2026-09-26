ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS grok_prompt_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS grok_prompt_mode VARCHAR(16) NOT NULL DEFAULT 'prepend',
    ADD COLUMN IF NOT EXISTS grok_prompt_content TEXT NOT NULL DEFAULT '';

ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_grok_prompt_mode_check;

ALTER TABLE groups
    ADD CONSTRAINT groups_grok_prompt_mode_check
    CHECK (grok_prompt_mode IN ('prepend', 'append'));
