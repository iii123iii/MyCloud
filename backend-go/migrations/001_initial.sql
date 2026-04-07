-- MyCloud — Initial Schema
-- MariaDB 11

SET NAMES utf8mb4;
SET time_zone = '+00:00';

-- ─── Users ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id              CHAR(36)        NOT NULL,
    username        VARCHAR(50)     NOT NULL,
    email           VARCHAR(255)    NOT NULL,
    password_hash   VARCHAR(255)    NOT NULL,
    role            ENUM('admin','user') NOT NULL DEFAULT 'user',
    quota_bytes     BIGINT          NOT NULL DEFAULT 10737418240, -- 10 GB
    used_bytes      BIGINT          NOT NULL DEFAULT 0,
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    must_change_password TINYINT(1) NOT NULL DEFAULT 0,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_username (username),
    UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Folders ──────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS folders (
    id          CHAR(36)    NOT NULL,
    name        VARCHAR(255) NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    parent_id   CHAR(36)    NULL DEFAULT NULL,
    is_deleted  TINYINT(1)  NOT NULL DEFAULT 0,
    deleted_at  DATETIME    NULL DEFAULT NULL,
    created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_folders_user_id (user_id),
    KEY idx_folders_parent_id (parent_id),
    CONSTRAINT fk_folders_user   FOREIGN KEY (user_id)   REFERENCES users   (id) ON DELETE CASCADE,
    CONSTRAINT fk_folders_parent FOREIGN KEY (parent_id) REFERENCES folders (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Files ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS files (
    id                  CHAR(36)        NOT NULL,
    name                VARCHAR(255)    NOT NULL,
    storage_path        VARCHAR(512)    NOT NULL,
    size_bytes          BIGINT          NOT NULL DEFAULT 0,
    mime_type           VARCHAR(127)    NOT NULL DEFAULT 'application/octet-stream',
    user_id             CHAR(36)        NOT NULL,
    folder_id           CHAR(36)        NULL DEFAULT NULL,
    encryption_key_enc  TEXT            NOT NULL,       -- per-file AES key, encrypted with master key (hex)
    encryption_iv       VARCHAR(32)     NOT NULL,       -- 12-byte GCM nonce (hex)
    encryption_tag      VARCHAR(32)     NOT NULL,       -- 16-byte GCM auth tag for the file key encryption (hex)
    is_deleted          TINYINT(1)      NOT NULL DEFAULT 0,
    deleted_at          DATETIME        NULL DEFAULT NULL,
    is_starred          TINYINT(1)      NOT NULL DEFAULT 0,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_files_user_id   (user_id),
    KEY idx_files_folder_id (folder_id),
    FULLTEXT KEY ft_files_name (name),
    CONSTRAINT fk_files_user   FOREIGN KEY (user_id)   REFERENCES users   (id) ON DELETE CASCADE,
    CONSTRAINT fk_files_folder FOREIGN KEY (folder_id) REFERENCES folders (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Shares ───────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS shares (
    id              CHAR(36)                NOT NULL,
    token           VARCHAR(64)             NOT NULL,
    file_id         CHAR(36)                NULL DEFAULT NULL,
    folder_id       CHAR(36)                NULL DEFAULT NULL,
    created_by      CHAR(36)                NOT NULL,
    permission      ENUM('read','write')    NOT NULL DEFAULT 'read',
    password_hash   VARCHAR(255)            NULL DEFAULT NULL,
    expires_at      DATETIME                NULL DEFAULT NULL,
    created_at      DATETIME                NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_shares_token (token),
    KEY idx_shares_created_by (created_by),
    CONSTRAINT fk_shares_file      FOREIGN KEY (file_id)    REFERENCES files   (id) ON DELETE CASCADE,
    CONSTRAINT fk_shares_folder    FOREIGN KEY (folder_id)  REFERENCES folders (id) ON DELETE CASCADE,
    CONSTRAINT fk_shares_created   FOREIGN KEY (created_by) REFERENCES users   (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Activity Log ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS activity_log (
    id              BIGINT          NOT NULL AUTO_INCREMENT,
    user_id         CHAR(36)        NULL DEFAULT NULL,
    action          VARCHAR(50)     NOT NULL,
    resource_type   VARCHAR(50)     NULL DEFAULT NULL,
    resource_id     CHAR(36)        NULL DEFAULT NULL,
    details         JSON            NULL DEFAULT NULL,
    ip_address      VARCHAR(45)     NULL DEFAULT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_activity_user_id    (user_id),
    KEY idx_activity_created_at (created_at),
    CONSTRAINT fk_activity_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Settings ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS settings (
    key_name    VARCHAR(100)    NOT NULL,
    value       TEXT            NULL DEFAULT NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (key_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed: mark setup as not complete
INSERT IGNORE INTO settings (key_name, value) VALUES ('setup_complete', 'false');
INSERT IGNORE INTO settings (key_name, value) VALUES ('registration_enabled', 'true');
INSERT IGNORE INTO settings (key_name, value) VALUES ('default_quota_bytes', '10737418240');
