-- Content-hash deduplication. files.content_sha256 stores the SHA-256 of
-- the plaintext. Two files with the same hash for the same user share an
-- on-disk blob via blob_refs.

ALTER TABLE files
    ADD COLUMN IF NOT EXISTS content_sha256 CHAR(64) NULL;

CREATE INDEX IF NOT EXISTS idx_files_user_hash ON files (user_id, content_sha256);

CREATE TABLE IF NOT EXISTS blob_refs (
    storage_path VARCHAR(512) NOT NULL,
    ref_count    INT          NOT NULL DEFAULT 1,
    PRIMARY KEY (storage_path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Backfill: count references for every existing storage_path across both files
-- and file_versions. Idempotent thanks to ON DUPLICATE KEY UPDATE.
INSERT INTO blob_refs (storage_path, ref_count)
SELECT storage_path, COUNT(*) AS n FROM (
    SELECT storage_path FROM files
    UNION ALL
    SELECT storage_path FROM file_versions
) AS combined
GROUP BY storage_path
ON DUPLICATE KEY UPDATE ref_count = VALUES(ref_count);
