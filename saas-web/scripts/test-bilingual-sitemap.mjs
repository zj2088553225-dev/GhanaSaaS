import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

const service = readFileSync(new URL('../../internal/service/storefront_service.go', import.meta.url), 'utf8')
const serviceTest = readFileSync(new URL('../../internal/service/storefront_service_test.go', import.meta.url), 'utf8')
const robots = readFileSync(new URL('../public/robots.txt', import.meta.url), 'utf8')
const indexHtml = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
const repoRoot = new URL('../..', import.meta.url)

assert.match(service, /XMLNSXHTML\s+string\s+`xml:"xmlns:xhtml,attr"`/)
assert.match(service, /XMLNSXHTML:\s*"http:\/\/www\.w3\.org\/1999\/xhtml"/)
assert.match(service, /localizedSitemapURLs\(baseURL,\s*"\/mall\?source=t6"/)
assert.match(service, /localizedSitemapURLs\(baseURL,\s*"\/mall\?source=merchant"/)
assert.match(service, /localizedSitemapURLs\(baseURL,\s*"\/merchant\/join"/)
assert.match(service, /localizedSitemapURLs\(baseURL,\s*"\/shops\/"\+url\.PathEscape\(slug\)/)
assert.match(service, /localizedSitemapURLs\(baseURL,\s*"\/products\/"\+url\.PathEscape\(BuildProductSEOPathSegment/)

for (const expected of [
  '<loc>https://www.ghanachinatown.com/en</loc>',
  '<loc>https://www.ghanachinatown.com/zh</loc>',
  '<loc>https://www.ghanachinatown.com/en/mall</loc>',
  '<loc>https://www.ghanachinatown.com/zh/mall</loc>',
  '<loc>https://www.ghanachinatown.com/en/mall?source=t6</loc>',
  '<loc>https://www.ghanachinatown.com/zh/mall?source=t6</loc>',
  '<loc>https://www.ghanachinatown.com/en/mall?source=merchant</loc>',
  '<loc>https://www.ghanachinatown.com/zh/mall?source=merchant</loc>',
  '<loc>https://www.ghanachinatown.com/en/merchant/join</loc>',
  '<loc>https://www.ghanachinatown.com/zh/merchant/join</loc>',
  '<loc>https://www.ghanachinatown.com/en/products/cruiser-motorcycle-10</loc>',
  '<loc>https://www.ghanachinatown.com/zh/products/cruiser-motorcycle-10</loc>',
  '<loc>https://www.ghanachinatown.com/en/shops/public-shop</loc>',
  '<loc>https://www.ghanachinatown.com/zh/shops/public-shop</loc>',
  'hreflang="en"',
  'hreflang="zh"',
  'hreflang="x-default"',
]) {
  assert.match(serviceTest, new RegExp(escapeRegExp(expected)), `${expected} should be asserted by Go sitemap test`)
}

for (const blocked of [
  '<loc>https://www.ghanachinatown.com/mall</loc>',
  '<loc>https://www.ghanachinatown.com/mall?source=t6</loc>',
  '<loc>https://www.ghanachinatown.com/products/cruiser-motorcycle-10</loc>',
  '<loc>https://www.ghanachinatown.com/shops/public-shop</loc>',
  '/admin',
  '/login',
  '/gc-control-7x',
  '/cart',
  '/checkout',
  '/orders',
]) {
  assert.match(serviceTest, new RegExp(escapeRegExp(blocked)), `${blocked} should be rejected by Go sitemap test`)
}

assert.match(robots, /Sitemap: https:\/\/www\.ghanachinatown\.com\/sitemap\.xml/)
assert.doesNotMatch(robots, /^Disallow:\s*\/en\/?\s*$/m)
assert.doesNotMatch(robots, /^Disallow:\s*\/zh\/?\s*$/m)
assert.match(robots, /Disallow: \/gc-control-7x\//)
assert.match(robots, /Disallow: \/admin\//)
assert.match(robots, /Disallow: \/login/)
assert.match(robots, /Disallow: \/cart/)
assert.match(robots, /Disallow: \/checkout/)
assert.match(robots, /Disallow: \/orders/)

assert.match(indexHtml, /<link rel="canonical" href="https:\/\/www\.ghanachinatown\.com\/en" \/>/)
assert.match(indexHtml, /<meta property="og:url" content="https:\/\/www\.ghanachinatown\.com\/en" \/>/)

execFileSync(
  'go',
  ['test', './internal/service', '-run', 'TestStorefrontServiceSitemapIncludesOnlyPublicShopsAndOnShelfProducts', '-count=1'],
  {
    cwd: repoRoot,
    env: { ...process.env, GOCACHE: process.env.GOCACHE || '/tmp/go-build-cache' },
    stdio: 'pipe',
  },
)

console.log('bilingual sitemap tests passed')

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
