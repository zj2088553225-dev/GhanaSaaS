import { defineStore } from 'pinia'
import { adminPath } from '../utils/adminPath.ts'

const AUTH_STORAGE_KEY = 'saas_admin_auth'
export const SUPER_BACKUP_TOKEN_KEY = 'saas_super_backup_token'
const SUPER_BACKUP_AUTH_KEY = 'saas_super_backup_auth'
const IMPERSONATION_TENANT_NAME_KEY = 'saas_impersonation_tenant_name'
const TENANT_SCOPED_ADMIN_STORAGE_KEYS = [
  'saas_admin_products_cache',
  'saas_admin_products_query',
]

export interface AuthUser {
  id: number
  tenant_id?: number
  role: string
  phone: string
  status: string
}

export interface AuthState {
  token: string
  user: AuthUser | null
  superBackupToken: string
  impersonatingTenantName: string
}

export interface AuthPayload {
  token: string
  user: AuthUser
}

export interface ImpersonateOptions {
  tenantName?: string
  redirectTo?: string
}

function readStoredAuth(): AuthState {
  const fallback: AuthState = {
    token: '',
    user: null,
    superBackupToken: getStoredSuperBackupToken(),
    impersonatingTenantName: getStoredImpersonationTenantName(),
  }

  try {
    const raw = localStorage.getItem(AUTH_STORAGE_KEY)
    if (!raw) {
      return fallback
    }

    const parsed = JSON.parse(raw) as Partial<AuthState>
    return {
      token: typeof parsed.token === 'string' ? parsed.token : '',
      user: parsed.user ?? null,
      superBackupToken: getStoredSuperBackupToken(),
      impersonatingTenantName: getStoredImpersonationTenantName(),
    }
  } catch {
    localStorage.removeItem(AUTH_STORAGE_KEY)
    return fallback
  }
}

function persistAuth(state: Pick<AuthState, 'token' | 'user'>) {
  if (!state.token || !state.user) {
    localStorage.removeItem(AUTH_STORAGE_KEY)
    return
  }

  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(state))
}

export function getStoredToken() {
  return readStoredAuth().token
}

export function getStoredUser() {
  return readStoredAuth().user
}

export function getStoredSuperBackupToken() {
  return localStorage.getItem(SUPER_BACKUP_TOKEN_KEY) || ''
}

function getStoredImpersonationTenantName() {
  return localStorage.getItem(IMPERSONATION_TENANT_NAME_KEY) || ''
}

function getStoredSuperBackupAuth(): AuthPayload | null {
  try {
    const raw = localStorage.getItem(SUPER_BACKUP_AUTH_KEY)
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw) as Partial<AuthPayload>
    if (typeof parsed.token === 'string' && parsed.user) {
      return { token: parsed.token, user: parsed.user }
    }
  } catch {
    localStorage.removeItem(SUPER_BACKUP_AUTH_KEY)
  }
  return null
}

function decodeUserFromToken(token: string): AuthUser | null {
  try {
    const payload = JSON.parse(atob(token.split('.')[1] || '')) as Partial<{
      user_id: number
      tenant_id: number
      role: string
    }>
    if (!payload.user_id || !payload.role) {
      return null
    }
    return {
      id: payload.user_id,
      tenant_id: payload.tenant_id,
      role: payload.role,
      phone: 'super_admin',
      status: 'active',
    }
  } catch {
    return null
  }
}

function clearStoredSuperBackup() {
  localStorage.removeItem(SUPER_BACKUP_TOKEN_KEY)
  localStorage.removeItem(SUPER_BACKUP_AUTH_KEY)
  localStorage.removeItem(IMPERSONATION_TENANT_NAME_KEY)
}

export function clearStoredAuth() {
  localStorage.removeItem(AUTH_STORAGE_KEY)
}

export function clearTenantScopedAdminState() {
  for (const key of TENANT_SCOPED_ADMIN_STORAGE_KEYS) {
    localStorage.removeItem(key)
    sessionStorage.removeItem(key)
  }
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => readStoredAuth(),
  getters: {
    isAuthenticated: (state) => Boolean(state.token),
    isImpersonating: (state) => Boolean(state.superBackupToken),
  },
  actions: {
    setAuth(payload: AuthPayload) {
      this.token = payload.token
      this.user = payload.user
      persistAuth({ token: this.token, user: this.user })
    },
    clearAuth() {
      this.token = ''
      this.user = null
      this.superBackupToken = ''
      this.impersonatingTenantName = ''
      clearStoredAuth()
      clearStoredSuperBackup()
    },
    logout() {
      this.clearAuth()
    },
    async impersonate(tenantId: number, options: ImpersonateOptions = {}) {
      if (!this.token || !this.user || this.user.role !== 'super_admin') {
        throw new Error('Only super admin can impersonate tenants')
      }

      const backup: AuthPayload = { token: this.token, user: this.user }
      localStorage.setItem(SUPER_BACKUP_TOKEN_KEY, backup.token)
      localStorage.setItem(SUPER_BACKUP_AUTH_KEY, JSON.stringify(backup))
      this.superBackupToken = backup.token
      this.impersonatingTenantName = options.tenantName || ''
      if (this.impersonatingTenantName) {
        localStorage.setItem(IMPERSONATION_TENANT_NAME_KEY, this.impersonatingTenantName)
      } else {
        localStorage.removeItem(IMPERSONATION_TENANT_NAME_KEY)
      }

      try {
        const { impersonateTenant } = await import('../api/superTenants')
        const result = await impersonateTenant(tenantId)
        clearTenantScopedAdminState()
        this.setAuth(result)
        this.impersonatingTenantName = options.tenantName || this.impersonatingTenantName
        const router = (await import('../router')).default
        await router.replace(options.redirectTo || adminPath('/dashboard'))
      } catch (error) {
        this.superBackupToken = ''
        this.impersonatingTenantName = ''
        clearStoredSuperBackup()
        throw error
      }
    },
    async exitImpersonation() {
      const backupToken = getStoredSuperBackupToken()
      const backup = getStoredSuperBackupAuth()
      const fallbackUser = backupToken ? decodeUserFromToken(backupToken) : null

      if (backup) {
        this.setAuth(backup)
      } else if (backupToken && fallbackUser) {
        this.setAuth({ token: backupToken, user: fallbackUser })
      } else {
        this.clearAuth()
      }

      clearStoredSuperBackup()
      this.superBackupToken = ''
      this.impersonatingTenantName = ''
      const router = (await import('../router')).default
      await router.replace(adminPath('/tenants'))
    },
  },
})
