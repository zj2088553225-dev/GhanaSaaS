\set ON_ERROR_STOP on

-- Read-only production preflight. Retain this output with the release record.
SELECT
    COUNT(*) AS total_events,
    COUNT(*) FILTER (
        WHERE traffic_type = 'human'
          AND event_name IN (
              'product_view',
              'product_click',
              'whatsapp_click',
              'phone_click'
          )
    ) AS popularity_events,
    MIN(occurred_at) AS oldest_event,
    MAX(occurred_at) AS newest_event
FROM storefront_events;

SELECT
    indexrelid::regclass AS index_name,
    indisvalid,
    indisready,
    pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
FROM pg_index
WHERE indexrelid = to_regclass('idx_storefront_events_popularity_human');
