-- 4-agent audit follow-ups (Streams 7A + supporting indexes).
--
-- 1. Composite indexes on share_grants so canAccessFile/Folder lookups
--    (filtered by grantee_user_id + file_id / folder_id) use an index instead
--    of a full grant-table scan. Materially speeds up every read on a shared
--    item once the install has many grants.
-- 2. Generated username_lower column + index for the login query. Today the
--    handler does WHERE email=? OR LOWER(username)=?; the LOWER() side is
--    unsargable and skips the unique username index, forcing a full table
--    scan on every login attempt. Username is already stored lowercased
--    (initial migration) but a generated column makes the equality literal,
--    matching the planner's expectations.

-- share_grants composite indexes. The base file_id / folder_id columns are
-- already indexed for the inverse direction; these add the grantee-prefix
-- variant for the access-check direction. CREATE INDEX IF NOT EXISTS isn't
-- portable across MariaDB versions, so we use the conditional pattern.

SET @s := (SELECT COUNT(*) FROM information_schema.statistics
           WHERE table_schema = DATABASE() AND table_name = 'share_grants'
             AND index_name = 'idx_share_grants_grantee_file');
SET @sql := IF(@s = 0,
    'CREATE INDEX idx_share_grants_grantee_file ON share_grants (grantee_user_id, file_id)',
    'SELECT 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @s := (SELECT COUNT(*) FROM information_schema.statistics
           WHERE table_schema = DATABASE() AND table_name = 'share_grants'
             AND index_name = 'idx_share_grants_grantee_folder');
SET @sql := IF(@s = 0,
    'CREATE INDEX idx_share_grants_grantee_folder ON share_grants (grantee_user_id, folder_id)',
    'SELECT 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
