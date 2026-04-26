-- Personal Access Tokens (PATs) for non-browser API clients.
--
-- The existing auth flow assumes a browser: GitHub OAuth issues a 1h JWT
-- (access cookie) and a 7d opaque refresh token (refresh cookie). That works
-- for the SPA but not for tools and skills that need to call the API
-- without a browser session. PATs are user-scoped long-lived bearer tokens
-- that authenticate to the same protected endpoints with the same Claims
-- shape, so every downstream handler keeps working unchanged.
--
-- Storage mirrors refresh_tokens: only the SHA-256 hex hash is persisted, the
-- raw token is shown to the user once at creation and never again. Revocation
-- is a soft set on revoked_at so we can keep an audit trail without breaking
-- foreign keys. expires_at is nullable; null means the token never expires
-- on its own (still revocable manually).

CREATE TABLE IF NOT EXISTS api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    last_used_at DATETIME,
    expires_at DATETIME,
    revoked_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
