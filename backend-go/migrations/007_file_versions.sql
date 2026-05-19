-- Historical revisions of a file's content. The current version stays on
-- the `files` row. A re-upload of a same-name file in the same folder
-- snapshots the previous current into this table and overwrites the files
-- row in place, keeping the file ID stable so comments/shares/tags survive.

CREATE TABLE IF NOT EXISTS file_versions (
    id                  CHAR(36)     NOT NULL,
    file_id             CHAR(36)     NOT NULL,
    version_no          INT          NOT NULL,
    storage_path        VARCHAR(512) NOT NULL,
    size_bytes          BIGINT       NOT NULL DEFAULT 0,
    encryption_key_enc  TEXT         NOT NULL,
    encryption_iv       VARCHAR(32)  NOT NULL,
    encryption_tag      VARCHAR(32)  NOT NULL,
    created_by          CHAR(36)     NOT NULL,
    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_versions_file_no (file_id, version_no),
    KEY idx_versions_file (file_id, version_no DESC),
    CONSTRAINT fk_versions_file FOREIGN KEY (file_id)    REFERENCES files (id) ON DELETE CASCADE,
    CONSTRAINT fk_versions_user FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
