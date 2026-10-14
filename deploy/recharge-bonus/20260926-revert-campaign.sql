-- Reverts only future-order campaign selection. NEVER rewrites existing order
-- snapshots, grants or user balances. Keep migration 254 and permanent-capable
-- application code after this rollback, because permanent promises may exist.
BEGIN;
SET LOCAL lock_timeout = '10s';
LOCK TABLE recharge_bonus_rules IN SHARE ROW EXCLUSIVE MODE;

DO $$
DECLARE
    campaign CONSTANT TEXT := 'kitty-20260926-permanent';
    original_rules JSONB;
    reverted TIMESTAMPTZ;
BEGIN
    SELECT rules_before, reverted_at INTO original_rules, reverted
    FROM recharge_bonus_campaign_backups WHERE campaign_key = campaign FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Campaign backup not found; refusing to guess previous rules';
    END IF;
    IF reverted IS NOT NULL THEN RETURN; END IF;

    -- Rows created for the campaign stay present but disabled to preserve any
    -- snapshot/grant foreign keys. Original rows retain their original IDs.
    UPDATE recharge_bonus_rules SET enabled = FALSE, updated_at = NOW()
    WHERE recharge_amount IN (5, 10, 30, 50, 100, 500, 1000, 5000)
      AND id NOT IN (SELECT b.id FROM jsonb_populate_recordset(NULL::recharge_bonus_rules, original_rules) b);

    UPDATE recharge_bonus_rules r
    SET bonus_amount = b.bonus_amount,
        validity_days = b.validity_days,
        enabled = b.enabled,
        starts_at = b.starts_at,
        ends_at = b.ends_at,
        updated_at = b.updated_at
    FROM jsonb_populate_recordset(NULL::recharge_bonus_rules, original_rules) b
    WHERE r.id = b.id AND r.recharge_amount = b.recharge_amount;

    UPDATE recharge_bonus_campaign_backups SET reverted_at = NOW()
    WHERE campaign_key = campaign;
END $$;
COMMIT;
