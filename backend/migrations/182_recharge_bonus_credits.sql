-- Recharge bonus credits are included in users.balance for compatibility, while
-- these tables retain ownership, expiry, and consumption order.

CREATE TABLE IF NOT EXISTS recharge_bonus_rules (
    id BIGSERIAL PRIMARY KEY,
    recharge_amount DECIMAL(20, 8) NOT NULL UNIQUE,
    bonus_amount DECIMAL(20, 8) NOT NULL,
    validity_days INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT recharge_bonus_rules_amount_positive CHECK (recharge_amount > 0),
    CONSTRAINT recharge_bonus_rules_bonus_positive CHECK (bonus_amount > 0),
    CONSTRAINT recharge_bonus_rules_validity_positive CHECK (validity_days > 0),
    CONSTRAINT recharge_bonus_rules_window_valid CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_rules_active
    ON recharge_bonus_rules (enabled, recharge_amount, starts_at, ends_at);

CREATE TABLE IF NOT EXISTS recharge_bonus_order_snapshots (
    payment_order_id BIGINT PRIMARY KEY REFERENCES payment_orders(id) ON DELETE CASCADE,
    rule_id BIGINT REFERENCES recharge_bonus_rules(id) ON DELETE SET NULL,
    recharge_amount DECIMAL(20, 8) NOT NULL,
    bonus_amount DECIMAL(20, 8) NOT NULL,
    validity_days INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT recharge_bonus_order_snapshot_amount_positive CHECK (recharge_amount > 0),
    CONSTRAINT recharge_bonus_order_snapshot_bonus_positive CHECK (bonus_amount > 0),
    CONSTRAINT recharge_bonus_order_snapshot_validity_positive CHECK (validity_days > 0)
);

CREATE TABLE IF NOT EXISTS recharge_bonus_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payment_order_id BIGINT NOT NULL UNIQUE REFERENCES payment_orders(id) ON DELETE CASCADE,
    rule_id BIGINT REFERENCES recharge_bonus_rules(id) ON DELETE SET NULL,
    granted_amount DECIMAL(20, 8) NOT NULL,
    remaining_amount DECIMAL(20, 8) NOT NULL,
    revoked_amount DECIMAL(20, 8) NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT recharge_bonus_grants_granted_positive CHECK (granted_amount > 0),
    CONSTRAINT recharge_bonus_grants_remaining_nonnegative CHECK (remaining_amount >= 0),
    CONSTRAINT recharge_bonus_grants_revoked_nonnegative CHECK (revoked_amount >= 0),
    CONSTRAINT recharge_bonus_grants_amounts_bounded CHECK (remaining_amount + revoked_amount <= granted_amount),
    CONSTRAINT recharge_bonus_grants_status_valid CHECK (status IN ('active', 'exhausted', 'expired', 'revoked'))
);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_grants_user_active_expiry
    ON recharge_bonus_grants (user_id, expires_at, id)
    WHERE remaining_amount > 0 AND status = 'active';

CREATE TABLE IF NOT EXISTS recharge_bonus_ledger (
    id BIGSERIAL PRIMARY KEY,
    grant_id BIGINT NOT NULL REFERENCES recharge_bonus_grants(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_id VARCHAR(255) NOT NULL,
    entry_type VARCHAR(20) NOT NULL,
    amount DECIMAL(20, 8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT recharge_bonus_ledger_amount_positive CHECK (amount > 0),
    CONSTRAINT recharge_bonus_ledger_type_valid CHECK (entry_type IN ('grant', 'consume', 'expire', 'revoke')),
    CONSTRAINT recharge_bonus_ledger_idempotent UNIQUE (grant_id, request_id, entry_type)
);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_ledger_user_created
    ON recharge_bonus_ledger (user_id, created_at DESC, id DESC);

-- Batch image jobs freeze balance before the final cost is known. Keep the
-- bonus source allocation so unused credit can only be restored while valid.
CREATE TABLE IF NOT EXISTS recharge_bonus_batch_holds (
    id BIGSERIAL PRIMARY KEY,
    batch_id VARCHAR(255) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    grant_id BIGINT NOT NULL REFERENCES recharge_bonus_grants(id) ON DELETE CASCADE,
    held_amount DECIMAL(20, 8) NOT NULL,
    consumed_amount DECIMAL(20, 8) NOT NULL DEFAULT 0,
    restored_amount DECIMAL(20, 8) NOT NULL DEFAULT 0,
    expired_amount DECIMAL(20, 8) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'held',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    settled_at TIMESTAMPTZ,
    CONSTRAINT recharge_bonus_batch_holds_unique UNIQUE (batch_id, grant_id),
    CONSTRAINT recharge_bonus_batch_holds_amount_positive CHECK (held_amount > 0),
    CONSTRAINT recharge_bonus_batch_holds_parts_nonnegative CHECK (
        consumed_amount >= 0 AND restored_amount >= 0 AND expired_amount >= 0
    ),
    CONSTRAINT recharge_bonus_batch_holds_parts_bounded CHECK (
        consumed_amount + restored_amount + expired_amount <= held_amount
    ),
    CONSTRAINT recharge_bonus_batch_holds_status_valid CHECK (status IN ('held', 'settled'))
);

CREATE INDEX IF NOT EXISTS idx_recharge_bonus_batch_holds_batch
    ON recharge_bonus_batch_holds (batch_id, status, id);

INSERT INTO recharge_bonus_rules (recharge_amount, bonus_amount, validity_days, enabled)
VALUES
    (30, 1, 1, TRUE),
    (50, 2, 3, TRUE),
    (100, 5, 10, TRUE),
    (300, 15, 15, TRUE),
    (500, 30, 30, TRUE),
    (1000, 50, 90, TRUE)
ON CONFLICT (recharge_amount) DO NOTHING;
