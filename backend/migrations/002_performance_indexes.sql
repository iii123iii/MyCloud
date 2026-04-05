-- Performance indexes for large-scale usage
-- Composite indexes for the most common query patterns

-- Files: list by folder (the primary dashboard query)
CREATE INDEX IF NOT EXISTS idx_files_user_folder_active
  ON files (user_id, folder_id, is_deleted)
  USING BTREE;

-- Files: starred listing across all folders
CREATE INDEX IF NOT EXISTS idx_files_user_starred
  ON files (user_id, is_starred, is_deleted)
  USING BTREE;

-- Files: recent files sorted by update time
CREATE INDEX IF NOT EXISTS idx_files_user_updated
  ON files (user_id, is_deleted, updated_at DESC)
  USING BTREE;

-- Files: trash listing
CREATE INDEX IF NOT EXISTS idx_files_user_deleted
  ON files (user_id, is_deleted, deleted_at)
  USING BTREE;

-- Folders: list by parent (navigation)
CREATE INDEX IF NOT EXISTS idx_folders_user_parent_active
  ON folders (user_id, parent_id, is_deleted)
  USING BTREE;

-- Folders: trash listing
CREATE INDEX IF NOT EXISTS idx_folders_user_deleted
  ON folders (user_id, is_deleted, deleted_at)
  USING BTREE;

-- Shares: lookup by token (public share resolution)
-- Already has UNIQUE on token, but add covering index
CREATE INDEX IF NOT EXISTS idx_shares_token_active
  ON shares (token)
  USING BTREE;

-- Activity log: admin queries by date
CREATE INDEX IF NOT EXISTS idx_activity_created_desc
  ON activity_log (created_at DESC)
  USING BTREE;
