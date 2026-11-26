# Storefront analytics reliability

## Event identity and migration

Every new storefront event must carry a client-generated UUID `event_id`. The
ID is created once when the business event enters the browser queue and remains
unchanged for normal retries, keepalive requests, and beacon delivery. A global
database unique index on `storefront_events.event_id` is the concurrency-safe
idempotency boundary. Conflict handling targets only that column; unrelated
database errors still fail the whole write.

Existing installations are upgraded without assuming `pgcrypto` or
`uuid-ossp`. The startup migration adds a nullable column, assigns UUIDs from Go
in bounded 500-row batches, runs a read-only missing/duplicate preflight, sets
PostgreSQL `NOT NULL`, and then creates the unique index. Historical rows are
classified as human because raw User-Agent values were intentionally never
stored. This migration is defined in code but was not run against production as
part of this release work.

## Basic bot classification

The public endpoint classifies a small fixed set of obvious crawler User-Agent
tokens such as `bot`, `crawler`, `spider`, `slurp`, and `headlesschrome`.
Unfamiliar or empty User-Agent values default to human to avoid aggressive
false positives; `curl` is not automatically classified as a bot. The full
User-Agent is never stored, and clients cannot choose `traffic_type`.

Bot events remain stored as `traffic_type=bot` for data-quality auditing. All
current overview, trend, source, product, and marketing-content analytics use a
shared `traffic_type=human` condition. This is basic crawler filtering, not a
claim to detect every bot, automation tool, or fraudulent visitor.

## Rate-limit boundary

The tracking endpoint applies both request-volume and event-volume limits. A
batch consumes one request allowance and one global/visitor event allowance per
event, so fifty-event batches are not equivalent to single events for volume
protection. The request allowance is acquired before the body is read or JSON
is decoded, so malformed payloads cannot bypass the global request boundary.
After decoding, invalid UTF-8 is rejected and the per-visitor/global event
counters are charged from the declared events. A global event counter remains
effective even if callers rotate visitor IDs.

The limiter is deliberately lightweight, in-memory, and process-local. In a
multi-instance deployment each application instance has an independent window;
there is no cluster-wide exact quota. This limitation avoids introducing Redis
or another distributed system in this phase and must be considered when sizing
edge or upstream rate limits.
