-- Increase the default user quota from 10 GB to 100 GB. For self-hosted
-- installs 10 GB fills up fast once versioning is in play. Existing users
-- still on the old default are bumped to the new one; users with a
-- custom quota are left alone.

ALTER TABLE users
    MODIFY COLUMN quota_bytes BIGINT NOT NULL DEFAULT 107374182400;

UPDATE users
    SET quota_bytes = 107374182400
    WHERE quota_bytes = 10737418240;

UPDATE settings
    SET value = '107374182400'
    WHERE key_name = 'default_quota_bytes' AND value = '10737418240';
