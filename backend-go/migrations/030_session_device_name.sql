-- Add an explicit, client-supplied device name to sessions.
--
-- The existing device_label is derived from the User-Agent by
-- deviceLabelFromUA (e.g. "Chrome on Windows"). That's fine for browsers but
-- coarse for the native apps, which know their own model. The QR device-link
-- flow lets the phone report a precise name ("Pixel 8") that we store here and
-- prefer over the UA-derived label when listing sessions.
--
-- IF NOT EXISTS so a partial prior apply doesn't block reapplication
-- (MariaDB 11 supports it; matches the idempotent pattern in 015).

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS device_name VARCHAR(120) NULL AFTER device_label;
