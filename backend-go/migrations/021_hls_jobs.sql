-- HLS transcode bookkeeping.
--
-- Tracks per-file HLS variants so the maintenance ticker can age out
-- entries when source files are deleted, and the admin dashboard can show
-- "X hours of video cached, Y MB used".

CREATE TABLE IF NOT EXISTS hls_jobs (
    file_id      CHAR(36)     NOT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'pending', -- pending|running|ready|failed
    error        TEXT         NULL,
    cache_dir    VARCHAR(512) NULL,
    bytes        BIGINT       NULL,
    duration_ms  INT          NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (file_id),
    KEY idx_hls_status (status),
    CONSTRAINT fk_hls_file FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
