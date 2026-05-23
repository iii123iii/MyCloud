-- Per-folder retention policies.
--
-- Each policy declares "files older than N days in this folder go to action X".
-- Hourly maintenance ticker reads these rows and applies the action.
--
-- action:
--   'soft_delete' — move files to trash (recoverable for the trash retention window)
--   'hard_delete' — permanent removal of file row + encrypted blob
--   'notify'      — write an activity_log row and a WS notification, leave files alone
--
-- A folder can have at most one active policy; replace by PUT.

CREATE TABLE IF NOT EXISTS retention_policies (
    folder_id      CHAR(36)            NOT NULL,
    retention_days INT                 NOT NULL,
    action         VARCHAR(16)         NOT NULL,
    created_by     CHAR(36)            NOT NULL,
    created_at     DATETIME            NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME            NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (folder_id),
    CONSTRAINT fk_retention_folder FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE,
    CONSTRAINT fk_retention_user   FOREIGN KEY (created_by) REFERENCES users(id)  ON DELETE CASCADE,
    CONSTRAINT ck_retention_days   CHECK (retention_days BETWEEN 1 AND 3650),
    CONSTRAINT ck_retention_action CHECK (action IN ('soft_delete','hard_delete','notify'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
