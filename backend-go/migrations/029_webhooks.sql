-- Outbound webhooks: per-user HTTP callbacks fired on the owner's own events.
--
-- The dispatcher signs each delivery with the webhook's secret (HMAC-SHA256,
-- Stripe-style "t=<unix>,v1=<hex>") so receivers can verify authenticity and
-- reject replays. During a secret rotation the previous_secret stays valid
-- until previous_secret_expires_at so receivers can cut over without dropping
-- deliveries; the maintenance sweep clears expired ones.
--
-- events: CSV of activity action names to deliver (e.g. "file.upload,share.create");
-- empty or "*" means all of the owner's events.

CREATE TABLE IF NOT EXISTS webhooks (
    id                         CHAR(36)      NOT NULL,
    user_id                    CHAR(36)      NOT NULL,
    url                        VARCHAR(2048) NOT NULL,
    secret                     VARCHAR(128)  NOT NULL,
    previous_secret            VARCHAR(128)  NULL,         -- valid during rotation grace
    previous_secret_expires_at DATETIME      NULL,
    events                     VARCHAR(1024) NOT NULL DEFAULT '', -- CSV of action names; "*"/empty = all
    is_active                  TINYINT(1)    NOT NULL DEFAULT 1,
    failure_count              INT           NOT NULL DEFAULT 0,
    last_delivery_at           DATETIME      NULL,
    created_at                 DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_webhooks_user_active (user_id, is_active),
    CONSTRAINT fk_webhooks_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              CHAR(36)    NOT NULL,
    webhook_id      CHAR(36)    NOT NULL,
    event_type      VARCHAR(64) NOT NULL,
    request_body    MEDIUMTEXT  NULL,          -- stored only for non-success deliveries (capped)
    response_status INT         NULL,
    error           VARCHAR(512) NULL,
    attempts        INT         NOT NULL DEFAULT 0,
    status          ENUM('pending','success','failed') NOT NULL DEFAULT 'pending',
    created_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at    DATETIME    NULL,
    PRIMARY KEY (id),
    KEY idx_deliveries_webhook (webhook_id, created_at),
    CONSTRAINT fk_deliveries_webhook FOREIGN KEY (webhook_id) REFERENCES webhooks (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
