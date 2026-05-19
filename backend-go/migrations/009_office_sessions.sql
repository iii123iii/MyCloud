-- OnlyOffice editing sessions.

CREATE TABLE IF NOT EXISTS office_sessions (
    id         CHAR(36)    NOT NULL,
    file_id    CHAR(36)    NOT NULL,
    user_id    CHAR(36)    NOT NULL,
    edit_key   VARCHAR(64) NOT NULL,
    created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_office_editkey (edit_key),
    KEY idx_office_file (file_id),
    CONSTRAINT fk_office_file FOREIGN KEY (file_id) REFERENCES files (id) ON DELETE CASCADE,
    CONSTRAINT fk_office_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
