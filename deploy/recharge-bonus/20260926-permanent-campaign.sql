-- Explicit Kitty campaign change, NOT an automatically applied migration.
-- Apply only after migration 254 and the permanent-capable application (and
-- compatible rollback image) are verified. Future order snapshots use this
-- campaign; existing snapshots, grants, balances and the 300 tier are untouched.
BEGIN;
SET LOCAL lock_timeout = '10s';
LOCK TABLE recharge_bonus_rules IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE IF NOT EXISTS recharge_bonus_campaign_backups (
    campaign_key TEXT PRIMARY KEY,
    rules_before JSONB NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reverted_at TIMESTAMPTZ
);

DO $$
DECLARE
    campaign CONSTANT TEXT := 'kitty-20260926-permanent';
BEGIN
    IF EXISTS (SELECT 1 FROM recharge_bonus_campaign_backups WHERE campaign_key = campaign) THEN
        IF EXISTS (SELECT 1 FROM recharge_bonus_campaign_backups
                   WHERE campaign_key = campaign AND reverted_at IS NOT NULL) THEN
            RAISE EXCEPTION 'This campaign was reverted; use a newly reviewed campaign to reactivate';
        END IF;
        -- Repeated deployment must not overwrite the original backup or edits.
        RETURN;
    END IF;

    INSERT INTO recharge_bonus_campaign_backups (campaign_key, rules_before)
    SELECT campaign, COALESCE(jsonb_agg(to_jsonb(r)), '[]'::jsonb)
    FROM recharge_bonus_rules r
    WHERE recharge_amount IN (5, 10, 30, 50, 100, 500, 1000, 5000);

    INSERT INTO recharge_bonus_rules
        (recharge_amount, bonus_amount, validity_days, enabled, starts_at, ends_at)
    VALUES (30, 1, 0, TRUE, NULL, NULL),
           (50, 1.99, 0, TRUE, NULL, NULL),
           (100, 4.99, 0, TRUE, NULL, NULL),
           (500, 29.99, 0, TRUE, NULL, NULL),
           (1000, 49.99, 0, TRUE, NULL, NULL),
           (5000, 249.99, 0, TRUE, NULL, NULL)
    ON CONFLICT (recharge_amount) DO UPDATE
    SET bonus_amount = EXCLUDED.bonus_amount,
        validity_days = EXCLUDED.validity_days,
        enabled = EXCLUDED.enabled,
        starts_at = NULL,
        ends_at = NULL,
        updated_at = NOW();

    UPDATE recharge_bonus_rules SET enabled = FALSE, updated_at = NOW()
    WHERE recharge_amount IN (5, 10) AND enabled;
END $$;
COMMIT;
