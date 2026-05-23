-- Persisted search queries per user.

CREATE TABLE IF NOT EXISTS saved_queries (
    id          CHAR(36)     NOT NULL,
    user_id     CHAR(36)     NOT NULL,
    name        VARCHAR(200) NOT NULL,
    query_text  TEXT         NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_saved_queries_user_name (user_id, name),
    KEY idx_saved_queries_user (user_id),
    CONSTRAINT fk_saved_queries_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
