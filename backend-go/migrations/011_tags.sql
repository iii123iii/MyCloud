-- User-defined coloured labels attached to files or folders.
-- (file_id, tag_id) and (folder_id, tag_id) are PRIMARY KEYs so duplicate
-- attachments are no-ops.

CREATE TABLE IF NOT EXISTS tags (
    id         CHAR(36)     NOT NULL,
    user_id    CHAR(36)     NOT NULL,
    name       VARCHAR(64)  NOT NULL,
    color      CHAR(7)      NOT NULL DEFAULT '#6b7280',
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_tags_user_name (user_id, name),
    CONSTRAINT fk_tags_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS file_tags (
    file_id CHAR(36) NOT NULL,
    tag_id  CHAR(36) NOT NULL,
    PRIMARY KEY (file_id, tag_id),
    KEY idx_filetags_tag (tag_id),
    CONSTRAINT fk_filetags_file FOREIGN KEY (file_id) REFERENCES files (id) ON DELETE CASCADE,
    CONSTRAINT fk_filetags_tag  FOREIGN KEY (tag_id)  REFERENCES tags  (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS folder_tags (
    folder_id CHAR(36) NOT NULL,
    tag_id    CHAR(36) NOT NULL,
    PRIMARY KEY (folder_id, tag_id),
    KEY idx_foldertags_tag (tag_id),
    CONSTRAINT fk_foldertags_folder FOREIGN KEY (folder_id) REFERENCES folders (id) ON DELETE CASCADE,
    CONSTRAINT fk_foldertags_tag    FOREIGN KEY (tag_id)    REFERENCES tags    (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
