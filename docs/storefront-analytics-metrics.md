# Storefront analytics metrics

This document is the contract for every storefront analytics API. Overview,
trend, source, product, and content reports must use the same definitions.
Business time is always based on `storefront_events.occurred_at`; `created_at`
is not an analytics timestamp.

## Identity and event semantics

- A `visitor` is anonymous and is not a signed-in user. It is identified by the
  browser-generated `gc_visitor_id` UUID.
- A `session` is identified by `gc_session`. The storefront tracker starts a
  new session after 30 minutes without activity.
- An `impression` is recorded only when a product or marketing item enters the
  visible viewport.
- A `click` is an active click on a product card or clickable marketing item.
- A `product_view` is recorded only after a real product detail request has
  completed successfully.
- A `contact_intent` currently means `whatsapp_click` plus `phone_click` only.
- There are currently no order, GMV, revenue, payment, or payment-conversion
  analytics metrics.

Only the Phase 1 event allowlist is included in reports:

`page_view`, `product_impression`, `product_click`, `product_view`,
`content_impression`, `content_click`, `whatsapp_click`, and `phone_click`.

All current reports also require `traffic_type=human`. Obvious crawler events
are retained separately for operations auditing but do not contribute to any
visitor, session, page, product, content, source, or contact-intent metric. The
classifier is intentionally basic and is not comprehensive fraud detection.

## Core metric definitions

| Metric | Definition |
| --- | --- |
| `visitors` | `COUNT(DISTINCT visitor_id)` across valid events |
| `sessions` | `COUNT(DISTINCT session_id)` across valid events |
| `page_views` | count of `page_view` |
| `product_impressions` | count of `product_impression` |
| `product_clicks` | count of `product_click` |
| `product_views` | count of `product_view` |
| `contact_intents` | count of `whatsapp_click` plus `phone_click` |
| `product_ctr` | `product_clicks / product_impressions` |
| `detail_view_rate` | `product_views / product_clicks` |
| `contact_intent_rate` | `contact_intents / product_views` |

Content CTR is `content_clicks / content_impressions`. Product report CTR and
the two downstream product rates use the same denominators as the core funnel.
Every rate is a JSON number in the range normally represented as `0` to `1`;
the API never formats a percentage string. A zero denominator returns `0`.

For period comparison, `change_rate` is `(value - previous_value) /
previous_value`. If both values are zero, it is `0`. If the previous value is
zero and the current value is positive, `change_rate` is JSON `null` because an
infinite percentage increase is not a meaningful claim.

## Date, timezone, and scope

Date inputs are inclusive local calendar dates. For example, `from=2026-07-01`
and `to=2026-07-14` in `Africa/Accra` query the UTC interval corresponding to
`[2026-07-01 00:00:00, 2026-07-15 00:00:00)` in that timezone. Query bounds are
always half-open; no `23:59:59.999999` boundary is used. The default timezone is
`Africa/Accra`, and the default range is the most recent seven calendar days,
including today.

Super-admin all-platform scope includes tenant events and explicit platform
scope events whose `tenant_id` is `NULL`. A selected tenant and every
tenant-admin report use exact `tenant_id = ...` scope and exclude platform
scope and all other tenants.

## Source context and attribution boundary

Phase 1 stores the source attached to each event when that event occurs. It
does not store an attribution ID, `click_id`, campaign attribution chain, or a
source chain across route changes. Source reports are therefore
**event-context analytics**, not strict first-touch, last-touch, or multi-touch
attribution.

Product impressions and product clicks retain their card source. Product views
usually have `product_detail` context, while contact events keep the context of
their own link. A source row describes where its events happened; it must not
be presented as proof that a home-floor click directly caused a later product
view or WhatsApp contact. The aggregate product funnel is a relationship among
overall event counts, not a claim of user-level conversion attribution.

Product `unique_visitors` is `COUNT(DISTINCT visitor_id)` across all product
events for that product in the requested range. Visitors are not added across
event types, and bucket-level unique visitor counts cannot be added to obtain a
range total. Product lists contain only products with events in the selected
range. Current off-shelf metadata remains visible, while a hard-deleted product
keeps its historical event row with a `Deleted product #<id>` fallback. Product
search applies only to current product names and derived SEO slugs; deleted
products do not match search text.

## Marketing content reports

Marketing content is identified by the stable CMS `tracking_key` stored as the
event's `entity_id`. English and Chinese storefront variants share that key, so
they are one content entity in lists and summaries. `locale` is only an
`en`/`zh`/`other` breakdown; it never creates a second content row.

Content impressions and clicks are historical facts in `storefront_events`.
Current English CMS JSON is a separate, best-effort metadata dimension. The
resolver prefers current published English metadata and falls back to current
English draft metadata. Draft-only content therefore has
`currently_exists=true` and `published=false`. If a tracking key is absent from
both current JSON documents, historical aggregates remain available with
`currently_exists=false`; analytics reads never recreate the key or modify CMS.

Content `unique_visitors` is `COUNT(DISTINCT visitor_id)` across that tracking
key's `content_impression` and `content_click` events. Clicks remain event
counts and are not deduplicated. Each trend bucket computes its own distinct
visitors, so bucket-level unique visitors must not be summed to obtain the
range-level unique visitor count.

The normal content-list path is grouped, sorted, counted, and paginated in SQL.
Search, content-type, and current-existence filters depend on current CMS JSON;
that path may filter and paginate only the already grouped `entity_id` rows in
Go. It never loads raw `StorefrontEvent` rows. An independent content dimension
or daily aggregate can be considered only if that aggregate entity set becomes
large in a future phase.
