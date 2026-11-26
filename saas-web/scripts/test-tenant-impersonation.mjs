import assert from 'node:assert/strict'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore, clearTenantScopedAdminState } from '../src/stores/auth.ts'
import {
  MULTIPLE_SELF_OPERATED_TENANTS_MESSAGE,
  explicitTenantID,
  selfOperatedEntryMessage,
  tenantImpersonationPath,
} from '../src/utils/tenantImpersonation.ts'

function createMemoryStorage() {
  const values = new Map()
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
    clear: () => values.clear(),
  }
}

globalThis.localStorage = createMemoryStorage()
globalThis.sessionStorage = createMemoryStorage()

assert.equal(tenantImpersonationPath(4), '/super/tenants/4/impersonate')
assert.equal(tenantImpersonationPath(1), '/super/tenants/1/impersonate')
assert.throws(() => tenantImpersonationPath(0), /Invalid tenant id/)

assert.equal(explicitTenantID('4'), 4)
assert.equal(explicitTenantID(['1']), 1)
assert.equal(explicitTenantID(''), null)

assert.equal(
  selfOperatedEntryMessage([
    { id: 1, name: 'Platform Self Operated', type: 'self_operated', status: 'active', created_at: '', updated_at: '' },
    { id: 4, name: 'YunFei Store', type: 'self_operated', status: 'active', created_at: '', updated_at: '' },
  ]),
  MULTIPLE_SELF_OPERATED_TENANTS_MESSAGE,
)

setActivePinia(createPinia())
const authStore = useAuthStore()
authStore.setAuth({
  token: 'tenant-4-token',
  user: { id: 40, tenant_id: 4, role: 'tenant_admin', phone: 'tadmin-4', status: 'active' },
})
assert.equal(JSON.parse(localStorage.getItem('saas_admin_auth')).user.tenant_id, 4)

localStorage.setItem('saas_admin_products_cache', 'old-products')
sessionStorage.setItem('saas_admin_products_query', 'old-query')
clearTenantScopedAdminState()
assert.equal(localStorage.getItem('saas_admin_products_cache'), null)
assert.equal(sessionStorage.getItem('saas_admin_products_query'), null)

console.log('tenant impersonation tests passed')
