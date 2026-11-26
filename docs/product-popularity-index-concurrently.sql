\set ON_ERROR_STOP on

-- Run directly through psql. CREATE INDEX CONCURRENTLY must not be wrapped in
-- an application migration transaction.
SET lock_timeout = '5s';
SET statement_timeout = '30min';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_storefront_events_popularity_human
ON storefront_events (tenant_id, product_id, occurred_at DESC, event_name)
WHERE traffic_type = 'human'
  AND event_name IN (
      'product_view',
      'product_click',
      'whatsapp_click',
      'phone_click'
  );

ANALYZE storefront_events;
