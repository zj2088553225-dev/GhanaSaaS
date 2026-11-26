import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { getActivePinia } from 'pinia'
import { clearStoredAuth, getStoredToken, useAuthStore } from '../stores/auth'
import { clearStoredCustomerAuth, getStoredCustomerToken, useCustomerAuthStore } from '../stores/customerAuth'
import { ADMIN_LOGIN_PATH } from './adminPath.ts'
import { currentLocale, localeFromPath, withLocalePath } from './localePath.ts'

export interface ApiResponse<T> {
  data?: T
  message?: string
  error?: string
  code?: string
}

export class APIRequestError<T = unknown> extends Error {
  status?: number
  code?: string
  data?: T
  retryAfterSeconds?: number

  constructor(message: string, options: {
    status?: number
    code?: string
    data?: T
    retryAfterSeconds?: number
  } = {}) {
    super(message)
    this.name = 'APIRequestError'
    this.status = options.status
    this.code = options.code
    this.data = options.data
    this.retryAfterSeconds = options.retryAfterSeconds
  }
}

export function isAPIRequestError<T = unknown>(error: unknown): error is APIRequestError<T> {
  return error instanceof APIRequestError
}

const request = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 10000,
})

function requestPath(url?: string) {
  const baseURL = request.defaults.baseURL || ''
  return url?.startsWith('/api/v1') ? url : `${baseURL}${url || ''}`
}

function isStorefrontRequest(url?: string) {
  const path = requestPath(url)
  return path.includes('/api/v1/storefront') ||
    path.includes('/api/v1/customer') ||
    path.includes('/api/v1/auth/register') ||
    path.includes('/api/v1/auth/me') ||
    path.includes('/api/v1/auth/password') ||
    path.includes('/api/v1/auth/logout')
}

function isPublicStorefrontAPIRequest(url?: string) {
  return requestPath(url).includes('/api/v1/storefront')
}

function isAdminRequest(url?: string) {
  const path = requestPath(url)
  return path.includes('/api/v1/admin') || path.includes('/api/v1/super') || (path.includes('/api/v1/auth') && !isStorefrontRequest(url))
}

function isLoginRequest(url?: string) {
  return requestPath(url).includes('/api/v1/auth/login')
}

function activeStorefrontLocale() {
  if (typeof window === 'undefined') {
    return null
  }
  return localeFromPath(window.location.pathname)
}

function attachStorefrontLocale(config: InternalAxiosRequestConfig) {
  if (!isPublicStorefrontAPIRequest(config.url)) {
    return
  }
  const locale = activeStorefrontLocale()
  if (!locale) {
    return
  }
  if (config.params instanceof URLSearchParams) {
    if (!config.params.has('locale')) {
      config.params.set('locale', locale)
    }
    return
  }
  const params = config.params && typeof config.params === 'object' ? { ...config.params } : {}
  if (!Object.prototype.hasOwnProperty.call(params, 'locale')) {
    config.params = { ...params, locale }
  }
}

async function redirectToLogin(loginPath: string, localizeStorefront = false) {
  const { default: router } = await import('../router')
  const currentRoute = router.currentRoute.value
  const targetPath = localizeStorefront ? withLocalePath(currentLocale(currentRoute), loginPath) : loginPath

  if (currentRoute.path !== targetPath) {
    await router.replace({ path: targetPath, query: { redirect: currentRoute.fullPath } })
  }
}

request.interceptors.request.use((config) => {
  const isStorefront = isStorefrontRequest(config.url)
  const isAdmin = isAdminRequest(config.url)
  const token = isStorefront ? getStoredCustomerToken() : isAdmin ? getStoredToken() : getStoredToken()

  attachStorefrontLocale(config)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  (response) => response,
  (error: AxiosError<ApiResponse<unknown>>) => {
    if (error.response?.status === 401) {
      if (isLoginRequest(error.config?.url)) {
        const message = error.response?.data?.error || error.message || 'Request failed'
        return Promise.reject(new APIRequestError(message, {
          status: error.response?.status,
          code: error.response?.data?.code,
          data: error.response?.data?.data,
        }))
      }
      const isStorefront = isStorefrontRequest(error.config?.url)
      const activePinia = getActivePinia()

      if (activePinia) {
        if (isStorefront) {
          useCustomerAuthStore().clearAuth()
        } else {
          useAuthStore().clearAuth()
        }
      } else if (isStorefront) {
        clearStoredCustomerAuth()
      } else {
        clearStoredAuth()
      }

      void redirectToLogin(isStorefront ? '/login' : ADMIN_LOGIN_PATH, isStorefront)
    }

    const message = error.response?.data?.error || error.message || 'Request failed'
    const retryAfter = Number(error.response?.headers?.['retry-after'])
    return Promise.reject(new APIRequestError(message, {
      status: error.response?.status,
      code: error.response?.data?.code,
      data: error.response?.data?.data,
      retryAfterSeconds: Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : undefined,
    }))
  },
)

export default request
