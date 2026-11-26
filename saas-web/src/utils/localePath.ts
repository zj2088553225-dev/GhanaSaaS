import type { LocationQueryRaw, RouteLocationNormalizedLoaded } from 'vue-router'
import { ADMIN_BASE_PATH, isAdminFrontendPath, isLegacyAdminEntryPath, normalizePath } from './adminPath.ts'

export const DEFAULT_LOCALE = 'en'
export const SUPPORTED_LOCALES = ['en', 'zh'] as const

export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

const LOCALE_PREFIX_PATTERN = /^\/(en|zh)(?=\/|$)/
const UNMANAGED_PREFIXES = ['/api', '/uploads', '/assets']
const UNMANAGED_PATHS = new Set(['/sitemap.xml', '/robots.txt', '/favicon.svg', '/favicon.ico'])

export function isSupportedLocale(value: unknown): value is SupportedLocale {
  return typeof value === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(value)
}

export function localeFromPath(path: string): SupportedLocale | null {
  const pathname = splitURLPath(path).pathname
  const match = pathname.match(LOCALE_PREFIX_PATTERN)
  return isSupportedLocale(match?.[1]) ? match[1] : null
}

export function currentLocale(route?: Pick<RouteLocationNormalizedLoaded, 'params' | 'path'> | null): SupportedLocale {
  const localeParam = route?.params?.locale
  const paramValue = Array.isArray(localeParam) ? localeParam[0] : localeParam
  if (isSupportedLocale(paramValue)) {
    return paramValue
  }
  return localeFromPath(route?.path || '') || DEFAULT_LOCALE
}

export function stripLocalePrefix(path: string) {
  const { pathname, suffix } = splitURLPath(path)
  const stripped = pathname.replace(LOCALE_PREFIX_PATTERN, '') || '/'
  return `${stripped}${suffix}`
}

export function withLocalePath(locale: SupportedLocale, path = '/', query?: LocationQueryRaw | null) {
  if (!shouldLocalizePath(path)) {
    return appendQuery(path, query)
  }

  const { pathname, suffix } = splitURLPath(stripLocalePrefix(path))
  const normalizedPath = pathname === '/' ? '' : pathname
  return appendQuery(`/${locale}${normalizedPath}${suffix}`, query)
}

export function ensureLocalePath(path: string, locale: SupportedLocale = DEFAULT_LOCALE) {
  if (!shouldLocalizePath(path)) {
    return path
  }
  return withLocalePath(locale, path)
}

export function switchLocalePath(route: Pick<RouteLocationNormalizedLoaded, 'fullPath' | 'path'>, targetLocale: SupportedLocale) {
  return withLocalePath(targetLocale, stripLocalePrefix(route.fullPath || route.path || '/'))
}

export function localizedPathForRoute(
  route: Pick<RouteLocationNormalizedLoaded, 'params' | 'path'>,
  path = '/',
  query?: LocationQueryRaw | null,
) {
  return withLocalePath(currentLocale(route), path, query)
}

export function storefrontLocaleParam(route?: Pick<RouteLocationNormalizedLoaded, 'params' | 'path'> | null) {
  return { locale: currentLocale(route) }
}

export function isLocalePrefixedPath(path: string) {
  return Boolean(localeFromPath(path))
}

export function isManagedStorefrontPath(path: string) {
  return shouldLocalizePath(path)
}

function shouldLocalizePath(path: string) {
  const trimmed = path.trim()
  if (!trimmed || trimmed.startsWith('#')) {
    return false
  }
  if (/^(?:https?:)?\/\//i.test(trimmed) || /^(?:mailto|tel):/i.test(trimmed)) {
    return false
  }
  if (!trimmed.startsWith('/')) {
    return false
  }

  const pathname = splitURLPath(trimmed).pathname
  const cleanPath = normalizePath(pathname)
  if (isAdminFrontendPath(cleanPath) || isLegacyAdminEntryPath(cleanPath)) {
    return false
  }
  if (cleanPath === ADMIN_BASE_PATH || UNMANAGED_PATHS.has(cleanPath)) {
    return false
  }
  return !UNMANAGED_PREFIXES.some((prefix) => cleanPath === prefix || cleanPath.startsWith(`${prefix}/`))
}

function appendQuery(path: string, query?: LocationQueryRaw | null) {
  if (!query || !Object.keys(query).length) {
    return path
  }

  const [withoutHash, hash = ''] = path.split('#')
  const [pathname, existingQuery = ''] = withoutHash.split('?')
  const params = new URLSearchParams(existingQuery)
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') {
      params.delete(key)
      continue
    }
    params.delete(key)
    const values = Array.isArray(value) ? value : [value]
    for (const item of values) {
      if (item !== undefined && item !== null && item !== '') {
        params.append(key, String(item))
      }
    }
  }

  const queryString = params.toString()
  return `${pathname}${queryString ? `?${queryString}` : ''}${hash ? `#${hash}` : ''}`
}

function splitURLPath(path: string) {
  const input = path || '/'
  const match = input.match(/^([^?#]*)(.*)$/)
  const pathname = match?.[1] || '/'
  const suffix = match?.[2] || ''
  return {
    pathname: pathname.startsWith('/') ? pathname : `/${pathname}`,
    suffix,
  }
}
