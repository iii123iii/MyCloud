-- Audit trail for issued pre-signed URLs.
--
-- Each row records one issuance: who issued, when, when it expires, the
-- file referenced. The token itself is NOT stored — it's reconstructable
-- from the row but a leaked DB doesn't immediately leak working URLs.

CREATE TABLE IF NOT EXISTS presign_audit (
    id          CHAR(36)  NOT NULL,
    file_id     CHAR(36)  NOT NULL,
    issued_by   CHAR(36)  NOT NULL,
    purpose     VARCHAR(32) NOT NULL,
    expires_at  DATETIME  NOT NULL,
    issued_at   DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at     DATETIME  NULL,
    PRIMARY KEY (id),
    KEY idx_presign_audit_file (file_id),
    KEY idx_presign_audit_user (issued_by),
    KEY idx_presign_audit_expires (expires_at),
    CONSTRAINT fk_presign_audit_file FOREIGN KEY (file_id)   REFERENCES files(id) ON DELETE CASCADE,
    CONSTRAINT fk_presign_audit_user FOREIGN KEY (issued_by) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
