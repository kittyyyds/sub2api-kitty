-- New managed files only. Existing canvas/New objects are never enrolled or deleted.
CREATE TABLE IF NOT EXISTS kitty_retained_media (
 id UUID PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id),
 source_key VARCHAR(64) NOT NULL,
 sha256 VARCHAR(64) NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('image','video')),
 name VARCHAR(200) NOT NULL,
 mime_type VARCHAR(80) NOT NULL,
 bytes BIGINT NOT NULL CHECK(bytes > 0 AND bytes <= 100663296),
 status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','deleting','deleted')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp() + INTERVAL '24 hours',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
 UNIQUE(user_id,source_key)
);
CREATE INDEX IF NOT EXISTS kitty_retained_media_owner ON kitty_retained_media(user_id,created_at DESC);
CREATE INDEX IF NOT EXISTS kitty_retained_media_expiry ON kitty_retained_media(expires_at) WHERE status='active';
CREATE TABLE IF NOT EXISTS kitty_media_storage_orders (
 order_id VARCHAR(128) PRIMARY KEY REFERENCES kitty_media_wallet_orders(order_id),
 user_id BIGINT NOT NULL REFERENCES users(id),
 asset_id UUID NOT NULL REFERENCES kitty_retained_media(id),
 days INTEGER NOT NULL CHECK(days BETWEEN 1 AND 365),
 amount NUMERIC(20,8) NOT NULL CHECK(amount > 0),
 before_expiry TIMESTAMPTZ NOT NULL,
 after_expiry TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp()
);
CREATE INDEX IF NOT EXISTS kitty_media_storage_orders_owner ON kitty_media_storage_orders(user_id,created_at DESC);
