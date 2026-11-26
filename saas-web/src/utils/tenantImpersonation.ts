import type { Tenant } from '../api/superTenants'

export const MULTIPLE_SELF_OPERATED_TENANTS_MESSAGE = '存在多个自营店，请从租户列表选择具体店铺进入'
export const SELECT_EXPLICIT_TENANT_MESSAGE = '请从租户列表选择具体店铺进入'
export const SELF_OPERATED_NOT_FOUND_MESSAGE = '未找到平台自营租户，请检查 seeder 或租户类型字段'

export function tenantImpersonationPath(tenantId: number) {
  if (!Number.isInteger(tenantId) || tenantId <= 0) {
    throw new Error('Invalid tenant id')
  }
  return `/super/tenants/${tenantId}/impersonate`
}

export function explicitTenantID(value: unknown) {
  const raw = Array.isArray(value) ? value[0] : value
  if (typeof raw !== 'string' && typeof raw !== 'number') {
    return null
  }
  const tenantId = Number(raw)
  return Number.isInteger(tenantId) && tenantId > 0 ? tenantId : null
}

export function isSelfOperatedTenant(tenant: Tenant) {
  return tenant.is_self_operated === true || tenant.isSelfOperated === true || tenant.type === 'self_operated'
}

export function isActiveTenant(tenant: Tenant) {
  return tenant.status === 'active'
}

export function selfOperatedEntryMessage(tenants: Tenant[]) {
  const activeSelfOperatedTenants = tenants.filter((tenant) => isSelfOperatedTenant(tenant) && isActiveTenant(tenant))
  if (activeSelfOperatedTenants.length > 1) {
    return MULTIPLE_SELF_OPERATED_TENANTS_MESSAGE
  }
  if (activeSelfOperatedTenants.length === 1) {
    return SELECT_EXPLICIT_TENANT_MESSAGE
  }
  return SELF_OPERATED_NOT_FOUND_MESSAGE
}
