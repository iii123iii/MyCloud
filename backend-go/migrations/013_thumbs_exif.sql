-- Image thumbnails + EXIF metadata.

ALTER TABLE files
    ADD COLUMN IF NOT EXISTS thumb_path VARCHAR(512) NULL,
    ADD COLUMN IF NOT EXISTS taken_at   DATETIME     NULL,
    ADD COLUMN IF NOT EXISTS width      INT          NULL,
    ADD COLUMN IF NOT EXISTS height     INT          NULL;

CREATE INDEX IF NOT EXISTS idx_files_taken_at ON files (user_id, taken_at);

CREATE TABLE IF NOT EXISTS file_exif (
    file_id   CHAR(36) NOT NULL,
    exif_json JSON     NOT NULL,
    PRIMARY KEY (file_id),
    CONSTRAINT fk_file_exif_file FOREIGN KEY (file_id) REFERENCES files (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
