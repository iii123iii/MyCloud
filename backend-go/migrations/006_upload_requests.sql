-- Token-protected anonymous upload links into a chosen folder. Files
-- become property of the request creator (created_by).

CREATE TABLE IF NOT EXISTS upload_requests (
    id            CHAR(36)    NOT NULL,
    token         VARCHAR(64) NOT NULL,
    folder_id     CHAR(36)    NULL DEFAULT NULL,
    created_by    CHAR(36)    NOT NULL,
    expires_at    DATETIME    NULL DEFAULT NULL,
    max_files     INT         NULL DEFAULT NULL,
    used_files    INT         NOT NULL DEFAULT 0,
    password_hash VARCHAR(255) NULL DEFAULT NULL,
    created_at    DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_uploadreq_token (token),
    KEY idx_uploadreq_created_by (created_by),
    CONSTRAINT fk_uploadreq_folder  FOREIGN KEY (folder_id)  REFERENCES folders (id) ON DELETE SET NULL,
    CONSTRAINT fk_uploadreq_created FOREIGN KEY (created_by) REFERENCES users   (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
