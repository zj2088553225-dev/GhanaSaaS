# OpenAPI Signature v2

OpenAPI v2 requests use HMAC-SHA256 and require these headers:

- `X-Signature-Version: v2`
- `X-App-Key: <tenant app key>`
- `X-Timestamp: <Unix seconds or milliseconds>`
- `X-Nonce: <random 16-128 character value>`
- `X-Signature: <lowercase hexadecimal HMAC-SHA256>`

The timestamp must be within five minutes of the API server clock. A nonce may
be used only once for an AppKey during that window. Generate a new random nonce
for every request; a UUID is suitable.

## Canonical payload

Join these eight values with a single line feed (`\n`) and no trailing line
feed:

```text
OPENAPI-HMAC-SHA256-V2
<AppKey>
<UPPERCASE HTTP method>
<canonical path>
<canonical query>
<lowercase request body SHA-256>
<X-Timestamp exactly as sent>
<X-Nonce exactly as sent>
```

Canonicalization rules:

1. Path is the request path only, beginning with `/`. Percent escapes use
   uppercase hexadecimal digits.
2. Parse query parameters, RFC 3986 percent-encode UTF-8 keys and values, sort
   encoded `key=value` pairs bytewise, then join them with `&`. A space is
   `%20`, never `+`. An empty query is an empty line.
3. Hash the exact request body bytes with SHA-256. The empty-body hash is
   `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
4. Compute `HMAC-SHA256(AppSecret, canonical_payload)` and send its lowercase
   hexadecimal representation as `X-Signature`.

Example canonical payload:

The body in this example is `[{"sku_code":"SKU-1","stock":8}]`.

```text
OPENAPI-HMAC-SHA256-V2
0123456789abcdef
POST
/api/v1/openapi/inventory/sync
dry_run=false&source=erp
a97249f134820a9cc19c31f350c54adea1f61a5b8a1d9d3e3a698f70b02113fc
1783670400
550e8400-e29b-41d4-a716-446655440000
```

The request body remains available to the endpoint after signature
verification.

## Node.js 20 example

```js
import crypto from 'node:crypto'

const appKey = process.env.OPENAPI_APP_KEY
const appSecret = process.env.OPENAPI_APP_SECRET
const method = 'POST'
const url = new URL('https://shop.example.com/api/v1/openapi/inventory/sync?source=erp&dry_run=false')
const body = Buffer.from('[{"sku_code":"SKU-1","stock":8}]')
const timestamp = Math.floor(Date.now() / 1000).toString()
const nonce = crypto.randomUUID()

const rfc3986 = (value) => encodeURIComponent(value)
  .replace(/[!'()*]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`)
const queryPairs = [...url.searchParams.entries()]
  .map(([key, value]) => `${rfc3986(key)}=${rfc3986(value)}`)
  .sort()
const canonicalPath = url.pathname.replace(/%[0-9a-f]{2}/gi, (value) => value.toUpperCase())
const bodyHash = crypto.createHash('sha256').update(body).digest('hex')
const payload = [
  'OPENAPI-HMAC-SHA256-V2',
  appKey,
  method,
  canonicalPath,
  queryPairs.join('&'),
  bodyHash,
  timestamp,
  nonce,
].join('\n')
const signature = crypto.createHmac('sha256', appSecret).update(payload).digest('hex')

const response = await fetch(url, {
  method,
  body,
  headers: {
    'Content-Type': 'application/json',
    'X-App-Key': appKey,
    'X-Timestamp': timestamp,
    'X-Nonce': nonce,
    'X-Signature-Version': 'v2',
    'X-Signature': signature,
  },
})
```

## Legacy compatibility

The legacy `sha256(AppKey + AppSecret + Timestamp)` signature does not bind the
method, path, query, body, or a nonce. It is disabled by default, including in
production. During a time-limited client migration it can be enabled explicitly
with `OPENAPI_LEGACY_SIGNATURE_ENABLED=true`; legacy clients should send
`X-Signature-Version: v1`. Disable the switch as soon as all clients use v2.
