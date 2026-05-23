-- Cooperative file locks for concurrent edit coordination.
--
-- Different code paths (OnlyOffice sessions, desktop sync writes,
-- HTTP PATCH /files/{id}) all need to know "is someone else editing
-- this right now?" before clobbering content. Existing office_sessions
-- handled the OnlyOffice case alone; this generalises to any kind of lock.
--
-- kind: 'office' (existing), 'sync' (desktop sync), 'edit' (web editor)
-- expires_at: heartbeat-refreshed; expired locks are ignored

CREATE TABLE IF NOT EXISTS file_locks (
    id          CHAR(36)    NOT NULL,
    file_id     CHAR(36)    NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    kind        VARCHAR(32) NOT NULL,
    expires_at  DATETIME    NOT NULL,
    created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    -- One active lock per (file, kind). A user re-acquiring refreshes;
    -- another user is rejected. Mixed-kind locks are allowed: a sync
    -- client can take 'sync' even while OnlyOffice holds 'office', and
    -- handlers decide compatibility.
    UNIQUE KEY uq_file_locks_file_kind (file_id, kind),
    KEY idx_file_locks_expires (expires_at),
    CONSTRAINT fk_file_locks_file FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
    CONSTRAINT fk_file_locks_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
