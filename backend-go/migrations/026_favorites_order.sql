-- Per-user ordered favorites (sidebar pins).
--
-- Distinct from is_starred on files: favourites are arbitrary folders the
-- user pinned in the sidebar for quick access, with a manual sort order.

CREATE TABLE IF NOT EXISTS user_favorites (
    user_id    CHAR(36) NOT NULL,
    folder_id  CHAR(36) NOT NULL,
    position   INT      NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, folder_id),
    KEY idx_user_favorites_user_position (user_id, position),
    CONSTRAINT fk_user_favorites_user   FOREIGN KEY (user_id)   REFERENCES users(id)   ON DELETE CASCADE,
    CONSTRAINT fk_user_favorites_folder FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
