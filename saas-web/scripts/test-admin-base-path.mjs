import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { ADMIN_BASE_PATH, ADMIN_LOGIN_PATH, adminPath } from '../src/utils/adminPath.ts'
import { isNoindexPath, routeSeoMeta } from '../src/composables/useSeoMeta.ts'

const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const adminLayout = readFileSync(new URL('../src/layouts/AdminLayout.vue', import.meta.url), 'utf8')
const authStore = readFileSync(new URL('../src/stores/auth.ts', import.meta.url), 'utf8')
const login = readFileSync(new URL('../src/views/Login.vue', import.meta.url), 'utf8')
const request = readFileSync(new URL('../src/utils/request.ts', import.meta.url), 'utf8')
const app = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const trackingHead = readFileSync(new URL('../src/components/storefront/StorefrontTrackingHead.vue', import.meta.url), 'utf8')
const robots = readFileSync(new URL('../public/robots.txt', import.meta.url), 'utf8')
const rootEnv = readFileSync(new URL('../../.env.production.example', import.meta.url), 'utf8')
const webEnv = readFileSync(new URL('../.env.production.example', import.meta.url), 'utf8')

assert.equal(ADMIN_BASE_PATH, '/gc-control-7x')
assert.equal(ADMIN_LOGIN_PATH, '/gc-control-7x/login')
assert.equal(adminPath('/dashboard'), '/gc-control-7x/dashboard')
assert.equal(adminPath('/products'), '/gc-control-7x/products')
assert.equal(adminPath('/tenants'), '/gc-control-7x/tenants')
assert.equal(adminPath('/merchant-leads'), '/gc-control-7x/merchant-leads')

assert.match(router, /path:\s*ADMIN_LOGIN_PATH/)
assert.match(router, /path:\s*ADMIN_BASE_PATH/)
assert.match(router, /path:\s*'\/login'/)
assert.match(router, /path:\s*'\/admin'/)
assert.match(router, /path:\s*'\/admin\/:pathMatch\(\.\*\)\*'/)
assert.doesNotMatch(router, /redirect:\s*'\/admin\//)
assert.doesNotMatch(router, /return\s+'\/admin\//)

assert.match(adminLayout, /adminPath\('\/products'\)/)
assert.match(adminLayout, /ADMIN_LOGIN_PATH/)
assert.doesNotMatch(adminLayout, /'\/admin\/products'/)
assert.doesNotMatch(adminLayout, /router\.replace\('\/login'\)/)

assert.match(authStore, /adminPath\('\/dashboard'\)/)
assert.match(authStore, /adminPath\('\/tenants'\)/)
assert.match(login, /adminPath\('\/global-dashboard'\)/)
assert.match(login, /isAdminFrontendPath/)
assert.match(request, /ADMIN_LOGIN_PATH/)
assert.doesNotMatch(request, /import\s+router\s+from\s+['"]\.\.\/router['"]/)
assert.match(request, /import\(['"]\.\.\/router['"]\)/)
assert.match(app, /isAdminFrontendPath/)
assert.match(trackingHead, /isAdminFrontendPath/)

assert.match(robots, /Disallow: \/gc-control-7x\//)
assert.match(rootEnv, /VITE_ADMIN_BASE_PATH=\/gc-control-7x/)
assert.match(webEnv, /VITE_ADMIN_BASE_PATH=\/gc-control-7x/)

for (const path of ['/gc-control-7x', '/gc-control-7x/login', '/gc-control-7x/dashboard', '/admin', '/admin/products', '/login']) {
  assert.equal(isNoindexPath(path), true, `${path} should be noindex`)
  assert.equal(routeSeoMeta(path).robots, 'noindex,nofollow,noarchive')
}

console.log('admin base path tests passed')
