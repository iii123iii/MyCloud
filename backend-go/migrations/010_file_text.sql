-- Plaintext extracted from supported file types. Indexed via MariaDB
-- FULLTEXT so queries use MATCH(...) AGAINST(...) IN NATURAL LANGUAGE MODE.

CREATE TABLE IF NOT EXISTS file_text (
    file_id    CHAR(36)    NOT NULL,
    content    MEDIUMTEXT  NOT NULL,
    indexed_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (file_id),
    FULLTEXT KEY ft_file_text_content (content),
    CONSTRAINT fk_file_text_file FOREIGN KEY (file_id) REFERENCES files (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
