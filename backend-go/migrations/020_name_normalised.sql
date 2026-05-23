-- Fuzzy filename search.
--
-- Adds a lowercase copy of the file name so fuzzy queries can match
-- "Réport-2025.pdf" against "report" via LIKE without depending on
-- case-folding behaviour of the active collation.
--
-- The real fuzzy work happens in the application layer
-- (internal/search/fuzzy.go: Damerau-Levenshtein scoring on a candidate
-- set returned by LIKE + the existing FULLTEXT on name). This migration
-- just provides the column and a regular FULLTEXT index for fast bare-
-- word lookups.
--
-- Idempotent (IF NOT EXISTS) so a partial first apply doesn't permanently
-- block reapplication: MariaDB doesn't have transactional DDL, and the
-- migration runner splits on `;` so each statement here must individually
-- tolerate already-applied state.
--
-- NOTE: an earlier draft used `WITH PARSER ngram` — that's MySQL-only;
-- MariaDB ships no built-in ngram parser. We rely on the application-side
-- scorer instead.

ALTER TABLE files
    ADD COLUMN IF NOT EXISTS name_normalised VARCHAR(512) NULL AFTER name;

UPDATE files SET name_normalised = LOWER(name) WHERE name_normalised IS NULL;

ALTER TABLE files
    ADD FULLTEXT INDEX IF NOT EXISTS ft_files_name_normalised (name_normalised);
