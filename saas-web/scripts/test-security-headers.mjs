import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const nginx = await readFile(new URL('../nginx.conf', import.meta.url), 'utf8')
const headers = await readFile(new URL('../security-headers.conf', import.meta.url), 'utf8')
const dockerfile = await readFile(new URL('../Dockerfile', import.meta.url), 'utf8')
const trackingEditor = await readFile(
  new URL('../src/views/admin/settings/StorefrontTracking.vue', import.meta.url),
  'utf8',
)

for (const header of [
  'Strict-Transport-Security',
  'X-Content-Type-Options',
  'Referrer-Policy',
  'Permissions-Policy',
  'X-Frame-Options',
  'Content-Security-Policy-Report-Only',
]) {
  assert.match(headers, new RegExp(header))
}

for (const origin of [
  'challenges.cloudflare.com',
  'www.google.com',
  'www.googletagmanager.com',
  'www.google-analytics.com',
  'connect.facebook.net',
]) {
  assert.match(headers, new RegExp(origin.replaceAll('.', '\\.')))
}

assert.doesNotMatch(headers, /add_header Content-Security-Policy "/)
assert.match(headers, /frame-ancestors 'none'/)
assert.match(nginx, /include \/etc\/nginx\/snippets\/security-headers\.conf;/)
assert.match(dockerfile, /COPY security-headers\.conf \/etc\/nginx\/snippets\/security-headers\.conf/)
assert.match(trackingEditor, /高风险权限/)
assert.match(trackingEditor, /只有 super_admin 有权保存/)

console.log('baseline security header tests passed')
