-- Slow-query log.
--
-- Captures DB queries that exceed the configured threshold (default 200 ms).
-- The maintenance ticker caps the table at the last 10 000 rows so growth
-- is bounded.

CREATE TABLE IF NOT EXISTS slow_queries (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    query_text  TEXT         NOT NULL,
    duration_ms INT          NOT NULL,
    rows_returned INT        NULL,
    captured_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_slow_queries_captured (captured_at),
    KEY idx_slow_queries_duration (duration_ms)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
