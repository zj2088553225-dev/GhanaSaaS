# Storefront analytics delivery

## Delivery contract

The storefront tracker keeps a short-lived in-memory queue and sends events to
`POST /api/v1/storefront/events/batch`. It flushes at 20 events or after two
seconds. The queue is deliberately not persisted in localStorage and is capped
at 500 events.

### Transport batch limits

The batch endpoint has a server request-body hard limit of 64 KiB and accepts
at most 50 events. Normal client delivery uses a smaller count limit of 20 and
a conservative 48 KiB delivery budget. The 48 KiB value is a project safety
margin, not a browser standard or hard limit.

The client measures the UTF-8 bytes of the final `{"events":[...]}` JSON body
with `TextEncoder`; JavaScript `string.length` is not a byte count. One shared
splitter enforces both the 20-event limit and the 48 KiB budget for normal,
Beacon, and keepalive delivery. A single unexpected event above that budget is
dropped with count-only development diagnostics so it cannot stall later queue
items. Under the current server field contract, one valid event is well below
48 KiB.

Backend `page_path` and `referrer` limits are Unicode rune/character limits:
512 and 1024 respectively. PostgreSQL `varchar(N)` likewise limits characters,
not UTF-8 bytes. Those field validity rules are separate from the transport
budget, which applies to the serialized request body.

Before persistence, the backend parses both URLs and removes query and fragment
data. Percent-encoded Unicode path characters are stored as readable Unicode;
ASCII reserved characters inside a path segment remain canonically
percent-escaped, so `a%2Fb` does not collapse into `a/b`. The exact normalized
strings are checked against the 512/1024 character limits before repository
writes. Referrer userinfo is rejected so credentials are never retained.

### Normal delivery

Normal delivery uses the in-memory queue and bounded retries. Only one byte-safe
normal batch is removed from the queue and placed in flight at a time. A failed
batch retains its exact composition for retry and is not merged with newer
queued events. Each business event receives one UUID `event_id` before it enters
the queue, and normal retries retain that ID. A request may reach the server
even when the browser never receives its response, so the transport is
duplicate-tolerant rather than exactly-once.

### Pagehide delivery

On `pagehide`, the tracker takes one snapshot containing the current in-flight
normal batch first and the remaining queued events second. It de-duplicates the
snapshot by `event_id` without changing those IDs, then uses the same byte-safe
splitter as normal delivery. The unresolved normal fetch is allowed to continue
while the snapshot is sent, so both requests may reach the server.

The snapshot prefers `navigator.sendBeacon` because the endpoint is public and
does not require Authorization. A `true` return value only means that the
browser accepted the data for transmission; it does not mean that the server
acknowledged persistence. If the browser rejects the beacon, the tracker falls
back to `fetch` with `keepalive: true`. Neither path blocks navigation or
displays an error to the shopper.

### Overall semantics

The overall contract is **in-memory best-effort delivery with bounded retries
and server-side idempotency**. It is not exactly-once and it is not guaranteed
at-least-once. The database `event_id` unique index makes duplicate deliveries
idempotent, but it cannot recover an event that never reaches the server.

When the queue reaches its memory cap, the client discards the oldest
impression events first, then page views, before sacrificing click, product
view, or contact-intent events. This only protects browser memory under extreme
load and does not change server-side metric definitions.

The queue lives only for the current page process. A browser crash can still
lose unsent events, and events are never retained for cross-day replay.
