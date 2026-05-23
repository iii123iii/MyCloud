-- Extend activity_log to support diff-based rollback.
--
-- Reversible operations (rename, move, soft-delete, share-create, tag-attach)
-- now snapshot the affected row's state before and after the change. An
-- admin endpoint reads these blobs and invokes a per-action rollback fn to
-- restore the prior state.
--
-- operation_id groups multi-step transactions: a batch move of 50 files
-- writes 50 activity rows sharing one operation_id so the admin can
-- "rollback the whole batch" without per-file clicks.
--
-- All ADDs use IF NOT EXISTS so a partial prior apply (any column in,
-- any other failing) doesn't block reapplication.

ALTER TABLE activity_log
    ADD COLUMN IF NOT EXISTS old_value JSON NULL AFTER details,
    ADD COLUMN IF NOT EXISTS new_value JSON NULL AFTER old_value,
    ADD COLUMN IF NOT EXISTS operation_id CHAR(36) NULL AFTER new_value,
    ADD COLUMN IF NOT EXISTS reversible TINYINT(1) NOT NULL DEFAULT 0 AFTER operation_id,
    ADD COLUMN IF NOT EXISTS rolled_back_at DATETIME NULL AFTER reversible,
    ADD INDEX IF NOT EXISTS idx_activity_log_operation_id (operation_id),
    ADD INDEX IF NOT EXISTS idx_activity_log_reversible (reversible, rolled_back_at);
