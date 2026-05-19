-- Per-file comment threads. Soft-delete via deleted_at so moderation
-- history is preserved.

CREATE TABLE IF NOT EXISTS comments (
    id          CHAR(36)    NOT NULL,
    file_id     CHAR(36)    NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    body        TEXT        NOT NULL,
    created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at  DATETIME    NULL DEFAULT NULL,
    PRIMARY KEY (id),
    KEY idx_comments_file (file_id, created_at),
    KEY idx_comments_user (user_id),
    CONSTRAINT fk_comments_file FOREIGN KEY (file_id) REFERENCES files (id) ON DELETE CASCADE,
    CONSTRAINT fk_comments_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
