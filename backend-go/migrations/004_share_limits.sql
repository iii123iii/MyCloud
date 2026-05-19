-- Per-share download cap + running counter. expires_at already exists from
-- 001_initial. Existing rows get download_limit=NULL (unlimited) and
-- download_count=0.

ALTER TABLE shares
    ADD COLUMN download_limit INT          NULL DEFAULT NULL,
    ADD COLUMN download_count INT NOT NULL DEFAULT 0;
