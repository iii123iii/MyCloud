-- Saved searches. Each row stores a user's query as JSON, runnable on demand.

CREATE TABLE IF NOT EXISTS smart_folders (
    id         CHAR(36)     NOT NULL,
    user_id    CHAR(36)     NOT NULL,
    name       VARCHAR(128) NOT NULL,
    query_json TEXT         NOT NULL,
    color      CHAR(7)      NULL DEFAULT NULL,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_smart_user_name (user_id, name),
    CONSTRAINT fk_smart_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
