import request, { type ApiResponse } from '../utils/request'
import type { AuthPayload } from '../stores/auth'
import { tenantImpersonationPath } from '../utils/tenantImpersonation'

export type TenantType = 'self_operated' | '3rd_party'
export type TenantStatus = 'active' | 'disabled'

export interface Tenant {
  id: number
  name: string
  type: TenantType
  status: TenantStatus
  is_self_operated?: boolean
  isSelfOperated?: boolean
  created_at: string
  updated_at: string
}

export interface TenantAdmin {
  id: number
  tenant_id?: number
  role: 'tenant_admin'
  phone: string
  status: string
  created_at: string
  updated_at: string
}

export interface CreateTenantPayload {
  name: string
  type?: TenantType
  status?: TenantStatus
  admin_phone: string
  admin_password: string
}

export interface CreateTenantResult {
  tenant: Tenant
  admin: TenantAdmin
}

export async function getSuperTenants(): Promise<Tenant[]> {
  const response = await request.get<ApiResponse<Tenant[]>>('/super/tenants')
  return response.data.data ?? []
}

export async function createSuperTenant(payload: CreateTenantPayload): Promise<CreateTenantResult> {
  const response = await request.post<ApiResponse<CreateTenantResult>>('/super/tenants', payload)
  if (!response.data.data) {
    throw new Error('Tenant creation failed')
  }
  return response.data.data
}

export async function impersonateTenant(tenantId: number): Promise<AuthPayload> {
  const response = await request.post<ApiResponse<AuthPayload>>(tenantImpersonationPath(tenantId))
  if (!response.data.data) {
    throw new Error('Impersonation failed')
  }
  return response.data.data
}
