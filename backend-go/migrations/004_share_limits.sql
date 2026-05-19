-- Per-share download cap + running counter. expires_at already exists from
-- 001_initial. Existing rows get download_limit=NULL (unlimited) and
-- download_count=0.

ALTER TABLE shares
    ADD COLUMN IF NOT EXISTS download_limit INT          NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS download_count INT NOT NULL DEFAULT 0;
