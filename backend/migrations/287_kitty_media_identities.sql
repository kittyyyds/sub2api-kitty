-- Stable identities for the private media bridge. This does not copy passwords,
-- issue login tokens, link by email, or enable media access for existing users.
CREATE TABLE IF NOT EXISTS kitty_media_identities (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    subject TEXT NOT NULL UNIQUE CHECK (subject ~ '^[A-Za-z0-9_-]{43}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
