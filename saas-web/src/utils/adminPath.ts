const DEFAULT_ADMIN_BASE_PATH = '/gc-control-7x'
const LEGACY_ADMIN_BASE_PATH = '/admin'
const adminEnvironment = typeof import.meta.env === 'object' ? import.meta.env : undefined

function normalizeBasePath(value: unknown) {
  const input = typeof value === 'string' ? value.trim() : ''
  if (!input) {
    return DEFAULT_ADMIN_BASE_PATH
  }

  const withLeadingSlash = input.startsWith('/') ? input : `/${input}`
  return withLeadingSlash.length > 1 && withLeadingSlash.endsWith('/')
    ? withLeadingSlash.slice(0, -1)
    : withLeadingSlash
}

export const ADMIN_BASE_PATH = normalizeBasePath(adminEnvironment?.VITE_ADMIN_BASE_PATH)
export const ADMIN_LOGIN_PATH = adminPath('/login')

export function adminPath(path = '') {
  const suffix = path.trim()
  if (!suffix || suffix === '/') {
    return ADMIN_BASE_PATH
  }
  return `${ADMIN_BASE_PATH}${suffix.startsWith('/') ? suffix : `/${suffix}`}`
}

export function isAdminFrontendPath(path: string) {
  const cleanPath = normalizePath(path)
  return cleanPath === ADMIN_BASE_PATH || cleanPath.startsWith(`${ADMIN_BASE_PATH}/`)
}

export function isLegacyAdminEntryPath(path: string) {
  const cleanPath = normalizePath(path)
  return cleanPath === LEGACY_ADMIN_BASE_PATH || cleanPath.startsWith(`${LEGACY_ADMIN_BASE_PATH}/`)
}

export function normalizePath(path: string) {
  const cleanPath = path.split('?')[0]?.split('#')[0] || '/'
  if (cleanPath.length > 1 && cleanPath.endsWith('/')) {
    return cleanPath.slice(0, -1)
  }
  return cleanPath || '/'
}
