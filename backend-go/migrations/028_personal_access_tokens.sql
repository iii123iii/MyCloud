-- Personal Access Tokens: long-lived, scoped API tokens for automation.
--
-- Authenticated as `Authorization: Bearer mc_pat_<random>`. The plaintext
-- token is shown to the user exactly once at creation; only the SHA-256 hash
-- is persisted. SHA-256 (not bcrypt) is deliberate: the token carries 256 bits
-- of entropy so it isn't brute-forceable, and a hash column can be UNIQUE-
-- indexed for O(1) lookup on the auth hot path — bcrypt would force a table
-- scan + slow compare per request.
--
-- scopes: sorted CSV of capabilities (e.g. "files:read,files:write"); the
-- literal "*" means full access and is stored ONLY when the user explicitly
-- chose it (never auto-collapsed from a fully-checked set).

CREATE TABLE IF NOT EXISTS personal_access_tokens (
    id           CHAR(36)     NOT NULL,
    user_id      CHAR(36)     NOT NULL,
    name         VARCHAR(100) NOT NULL,
    token_hash   CHAR(64)     NOT NULL,             -- hex(sha256(token))
    token_prefix VARCHAR(16)  NOT NULL,             -- display hint, e.g. "mc_pat_ab12"
    last_four    CHAR(4)      NOT NULL,             -- display hint, last 4 chars
    scopes       VARCHAR(255) NOT NULL DEFAULT '',  -- sorted CSV; "*" only if explicitly chosen
    last_used_at DATETIME     NULL,
    last_used_ip VARCHAR(64)  NULL,
    expires_at   DATETIME     NULL,
    revoked_at   DATETIME     NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_pat_token_hash (token_hash),
    KEY idx_pat_user (user_id),
    CONSTRAINT fk_pat_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
