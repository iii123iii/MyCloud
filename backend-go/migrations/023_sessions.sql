-- Per-device session bookkeeping.
--
-- Each issued JWT access/refresh pair gets a row keyed by jti. The
-- /me/sessions endpoint reads from here for the UI; /me/sessions/{jti}
-- DELETE revokes by both deleting the row AND adding bl:<jti> to Redis
-- for fast middleware checks.

CREATE TABLE IF NOT EXISTS sessions (
    jti          CHAR(36)     NOT NULL,
    user_id      CHAR(36)     NOT NULL,
    device_label VARCHAR(200) NULL,
    user_agent   VARCHAR(500) NULL,
    ip_address   VARCHAR(64)  NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   DATETIME     NULL,
    PRIMARY KEY (jti),
    KEY idx_sessions_user (user_id),
    KEY idx_sessions_expires (expires_at),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
