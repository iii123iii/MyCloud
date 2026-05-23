-- One-time-view share tokens.
--
-- single_view=1 means the share self-destructs (logically) after a single
-- successful download. The downloader race is closed atomically via
-- conditional UPDATE in handlers_shares.go — first request to successfully
-- bump download_count from 0 to 1 gets the file; everyone else sees 410 Gone.
--
-- Idempotent: both columns guarded by IF NOT EXISTS.

ALTER TABLE shares
    ADD COLUMN IF NOT EXISTS single_view TINYINT(1) NOT NULL DEFAULT 0 AFTER download_count,
    ADD COLUMN IF NOT EXISTS viewed_at   DATETIME   NULL                AFTER single_view;
