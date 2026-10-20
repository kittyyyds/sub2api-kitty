-- Capability only. No account links, balance copies or media access are enabled.
-- All money is SUB USD-labelled credit: recharge principal CNY 1 -> credit 1.
CREATE TABLE IF NOT EXISTS kitty_media_wallet_orders (
    order_id VARCHAR(128) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    fingerprint VARCHAR(64) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
    hold_amount NUMERIC(20,8) NOT NULL CHECK (hold_amount >= 0 AND hold_amount < 1000000),
    actual_amount NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (actual_amount >= 0),
    released_amount NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (released_amount >= 0),
    expired_amount NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (expired_amount >= 0),
    billing_state TEXT NOT NULL DEFAULT 'held' CHECK (billing_state IN ('held','captured','released')),
    submission_state TEXT NOT NULL DEFAULT 'reserved' CHECK (submission_state IN ('reserved','submitting','unknown','accepted','rejected')),
    upstream_id TEXT NOT NULL DEFAULT '',
    evidence_hash VARCHAR(64) NOT NULL DEFAULT '',
    settlement_hash VARCHAR(64) NOT NULL DEFAULT '',
    quote JSONB NOT NULL,
    price_contract JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((billing_state = 'held' AND actual_amount = 0 AND released_amount = 0 AND expired_amount = 0)
        OR (billing_state <> 'held' AND actual_amount + released_amount + expired_amount = hold_amount)),
    CHECK (billing_state <> 'released' OR actual_amount = 0)
);
CREATE INDEX IF NOT EXISTS kitty_media_wallet_user_idx ON kitty_media_wallet_orders(user_id, created_at);
CREATE INDEX IF NOT EXISTS kitty_media_wallet_pending_idx ON kitty_media_wallet_orders(updated_at) WHERE billing_state = 'held';

CREATE TABLE IF NOT EXISTS kitty_media_wallet_ledger (
    order_id VARCHAR(128) NOT NULL REFERENCES kitty_media_wallet_orders(order_id),
    entry_type TEXT NOT NULL CHECK (entry_type IN ('hold','capture','release')),
    held_amount NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (held_amount >= 0),
    actual_amount NUMERIC(20,8) NOT NULL CHECK (actual_amount >= 0),
    released_amount NUMERIC(20,8) NOT NULL CHECK (released_amount >= 0),
    expired_amount NUMERIC(20,8) NOT NULL CHECK (expired_amount >= 0),
    evidence_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (order_id, entry_type)
);
