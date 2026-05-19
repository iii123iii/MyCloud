-- Per-user share grants. An owner can give viewer/editor/owner access to
-- another MyCloud user on a single file OR a single folder. Mutually
-- exclusive target columns mirror the existing `shares` table convention.

CREATE TABLE IF NOT EXISTS share_grants (
    id              CHAR(36)    NOT NULL,
    file_id         CHAR(36)    NULL DEFAULT NULL,
    folder_id       CHAR(36)    NULL DEFAULT NULL,
    grantee_user_id CHAR(36)    NOT NULL,
    granted_by      CHAR(36)    NOT NULL,
    permission      ENUM('viewer','editor','owner') NOT NULL DEFAULT 'viewer',
    created_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_grants_file_grantee   (file_id, grantee_user_id),
    UNIQUE KEY uq_grants_folder_grantee (folder_id, grantee_user_id),
    KEY idx_grants_grantee    (grantee_user_id),
    KEY idx_grants_granted_by (granted_by),
    CONSTRAINT fk_grants_file    FOREIGN KEY (file_id)         REFERENCES files   (id) ON DELETE CASCADE,
    CONSTRAINT fk_grants_folder  FOREIGN KEY (folder_id)       REFERENCES folders (id) ON DELETE CASCADE,
    CONSTRAINT fk_grants_grantee FOREIGN KEY (grantee_user_id) REFERENCES users   (id) ON DELETE CASCADE,
    CONSTRAINT fk_grants_granted FOREIGN KEY (granted_by)      REFERENCES users   (id) ON DELETE CASCADE,
    CONSTRAINT chk_grants_target CHECK (
        (file_id IS NOT NULL AND folder_id IS NULL)
        OR (file_id IS NULL AND folder_id IS NOT NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
