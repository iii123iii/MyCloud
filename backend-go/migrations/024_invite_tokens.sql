-- Admin-issued invitation tokens.
--
-- An admin invokes POST /admin/invites to generate a signed token + URL.
-- The recipient visits /invite/{token} and submits an email + password;
-- the account is created with the role/quota baked into the token.
--
-- redeemed_by_user_id stays NULL until first use, then carries the new
-- user's id. Tokens with redeemed_by set cannot be re-used.

CREATE TABLE IF NOT EXISTS invite_tokens (
    token              CHAR(64)     NOT NULL,
    invited_email      VARCHAR(320) NULL,
    role               VARCHAR(16)  NOT NULL DEFAULT 'user',
    quota_bytes        BIGINT       NOT NULL,
    created_by         CHAR(36)     NOT NULL,
    created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at         DATETIME     NOT NULL,
    redeemed_at        DATETIME     NULL,
    redeemed_by_user_id CHAR(36)    NULL,
    PRIMARY KEY (token),
    KEY idx_invite_tokens_expires (expires_at),
    KEY idx_invite_tokens_redeemed (redeemed_at),
    CONSTRAINT fk_invite_tokens_creator FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
