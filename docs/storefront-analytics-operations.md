# Storefront analytics operations

## Current retention policy

Raw rows in `storefront_events` are retained by default. This release does not
schedule, trigger, or otherwise perform an automatic purge. The table is the
only historical fact store behind the current dashboards; no daily aggregate
table exists. Deleting old raw events now would therefore permanently remove
the corresponding Overview, Product, Content, Source, locale, and device
history.

The super-admin `GET /api/v1/super/analytics/status` endpoint provides aggregate
operational signals: total, human, bot, and last-24-hour event counts; oldest
and latest event timestamps; and counts by allowlisted event name. It never
returns visitor IDs, session IDs, referrers, or event rows. Database byte size
is intentionally omitted from the cross-dialect API because reliable table and
index sizing is PostgreSQL-specific.

## Read-only PostgreSQL capacity checks

The following statements are read-only. Run them against the intended database
with an appropriately restricted account and record the observation time.

```sql
SELECT
    COUNT(*) AS exact_rows,
    MIN(occurred_at) AS oldest_event_at,
    MAX(occurred_at) AS latest_event_at
FROM storefront_events;

SELECT
    pg_size_pretty(pg_relation_size('storefront_events')) AS table_size,
    pg_size_pretty(pg_indexes_size('storefront_events')) AS index_size,
    pg_size_pretty(pg_total_relation_size('storefront_events')) AS total_size;

SELECT event_name, traffic_type, COUNT(*)
FROM storefront_events
GROUP BY event_name, traffic_type
ORDER BY event_name, traffic_type;
```

For query investigations, capture `EXPLAIN (ANALYZE, BUFFERS)` on the exact
parameter shape used in production, using a safe read-only session. Do not infer
an aggregate schema solely from row count.

## Manual deletion example — high risk

There is currently no routine reason to execute this example. It is documented
only for a future, explicitly approved incident or retention change. Take and
verify a backup, confirm the cutoff in the business timezone, estimate affected
rows with `SELECT COUNT(*)`, and use a transaction. Without a durable aggregate,
the deleted dashboard history cannot be reconstructed.

```sql
BEGIN;

SELECT COUNT(*)
FROM storefront_events
WHERE occurred_at < TIMESTAMPTZ '2025-01-01 00:00:00+00';

-- HIGH RISK: enable only after approval, backup verification, and review.
-- DELETE FROM storefront_events
-- WHERE occurred_at < TIMESTAMPTZ '2025-01-01 00:00:00+00';

ROLLBACK; -- replace with COMMIT only under the approved runbook
```

A future policy might retain raw events for 180 or 365 days and retain verified
aggregates longer. That policy must not be enabled until the aggregate can
faithfully support every required historical report and has been reconciled
against raw data.

## Why there is no daily aggregate yet

Operations should first observe:

- `storefront_events` row count and table/index size;
- Overview query p95;
- Product List query p95;
- Content List query p95;
- requested date-range distribution;
- PostgreSQL execution plans and buffer usage.

Tens of millions of events or analytics query p95 materially exceeding the
product target may justify an aggregate investigation, but neither is a formal
SLA by itself. The decision must be based on representative
`EXPLAIN (ANALYZE, BUFFERS)` evidence.

An aggregate design would need to re-evaluate hour/date, tenant, event name,
product, content, source, locale, and device dimensions. Pre-creating a daily
table now could cause a dimension explosion or make existing hour, source, and
breakdown reports impossible to answer. Daily aggregation is explicitly not
implemented in this phase.
