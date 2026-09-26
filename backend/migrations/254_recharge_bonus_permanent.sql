-- Capability only: validity_days = 0 and a NULL grant expiry mean permanent
-- credit. Do not change campaign prices, existing snapshots, grants or balances.
-- Keep this widened schema when reverting the UI. Once permanent snapshots or
-- grants exist, every serving/rollback binary must retain permanent support.

ALTER TABLE recharge_bonus_rules
    DROP CONSTRAINT IF EXISTS recharge_bonus_rules_validity_positive;
ALTER TABLE recharge_bonus_rules
    DROP CONSTRAINT IF EXISTS recharge_bonus_rules_validity_nonnegative;
ALTER TABLE recharge_bonus_rules
    ADD CONSTRAINT recharge_bonus_rules_validity_nonnegative CHECK (validity_days >= 0);

ALTER TABLE recharge_bonus_order_snapshots
    DROP CONSTRAINT IF EXISTS recharge_bonus_order_snapshot_validity_positive;
ALTER TABLE recharge_bonus_order_snapshots
    DROP CONSTRAINT IF EXISTS recharge_bonus_order_snapshot_validity_nonnegative;
ALTER TABLE recharge_bonus_order_snapshots
    ADD CONSTRAINT recharge_bonus_order_snapshot_validity_nonnegative CHECK (validity_days >= 0);

ALTER TABLE recharge_bonus_grants ALTER COLUMN expires_at DROP NOT NULL;

COMMENT ON COLUMN recharge_bonus_rules.validity_days IS
    '0 = permanent credit; positive = days from fulfillment. Snapshotted at order creation.';
COMMENT ON COLUMN recharge_bonus_order_snapshots.validity_days IS
    'Immutable order-time duration: 0 = permanent; positive = days from fulfillment.';
COMMENT ON COLUMN recharge_bonus_grants.expires_at IS
    'NULL = permanent credit. Existing finite grant expiry is never rewritten by a rule change.';
