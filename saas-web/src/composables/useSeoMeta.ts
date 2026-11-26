import { onBeforeUnmount, unref, watchEffect, type MaybeRef } from 'vue'
import { ADMIN_BASE_PATH } from '../utils/adminPath.ts'
import {
  DEFAULT_LOCALE,
  SUPPORTED_LOCALES,
  localeFromPath,
  stripLocalePrefix,
  withLocalePath,
  type SupportedLocale,
} from '../utils/localePath.ts'

export const SITE_NAME = 'Ghana Chinatown'
export const SITE_ORIGIN = 'https://www.ghanachinatown.com'
export const DEFAULT_DESCRIPTION = 'Ghana Chinatown is a marketplace for products, shops, and cross-border retail services in Ghana.'
export const DEFAULT_IMAGE = `${SITE_ORIGIN}/favicon.svg`

export interface SeoMetaInput {
  title: string
  description?: string
  path?: string
  canonical?: string
  alternates?: SeoAlternateLinks | null
  robots?: string
  type?: string
  image?: string
  jsonLd?: unknown[]
}

export interface SeoAlternateLinks {
  en: string
  zh: string
  xDefault: string
}

type MallSeoSource = 'all' | 't6' | 'merchant'

const NOINDEX_PREFIXES = [ADMIN_BASE_PATH, '/admin']
const NOINDEX_PATHS = new Set([
  '/403',
  '/login',
  '/register',
  '/forgot-password',
  '/reset-password',
  '/account',
  '/cart',
  '/checkout',
  '/my-orders',
  '/orders',
  '/orders/success',
  '/order-success',
  '/store-login',
  '/under-construction',
  '/reservations/mine',
])

export function isNoindexPath(path: string) {
  const cleanPath = normalizePath(path)
  const storefrontPath = normalizePath(stripLocalePrefix(cleanPath))
  return (
    NOINDEX_PATHS.has(cleanPath) ||
    NOINDEX_PATHS.has(storefrontPath) ||
    NOINDEX_PREFIXES.some((prefix) => cleanPath === prefix || cleanPath.startsWith(`${prefix}/`))
  )
}

export function canonicalURL(path = '/') {
  const cleanPath = normalizeCanonicalPath(path)
  return cleanPath === '/' ? `${SITE_ORIGIN}/` : `${SITE_ORIGIN}${cleanPath}`
}

export function absoluteURL(value?: string | null) {
  const input = value?.trim()
  if (!input) {
    return DEFAULT_IMAGE
  }
  if (/^https?:\/\//i.test(input)) {
    return input
  }
  return `${SITE_ORIGIN}${input.startsWith('/') ? input : `/${input}`}`
}

export function plainText(value?: string | null, maxLength = 160) {
  const text = (value || '')
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (text.length <= maxLength) {
    return text
  }
  return `${text.slice(0, maxLength - 1).trim()}...`
}

export function routeSeoMeta(path: string): SeoMetaInput {
  const cleanPath = normalizePath(path)
  const storefrontPath = normalizePath(stripLocalePrefix(cleanPath))
  const locale = localeFromPath(cleanPath) || DEFAULT_LOCALE
  if (isNoindexPath(cleanPath)) {
    return {
      title: `${SITE_NAME}`,
      description: DEFAULT_DESCRIPTION,
      canonical: canonicalURL(cleanPath),
      alternates: null,
      robots: 'noindex,nofollow,noarchive',
    }
  }
  if (storefrontPath === '/') {
    return homeSeoMeta(locale)
  }
  if (storefrontPath === '/mall') {
    return mallSeoMeta(mallSeoSourceFromPath(path), locale)
  }
  if (storefrontPath === '/merchant/join') {
    return merchantJoinSeoMeta(locale)
  }
  return {
    title: SITE_NAME,
    description: DEFAULT_DESCRIPTION,
    canonical: canonicalURL(cleanPath),
    alternates: localizedAlternates(cleanPath),
    robots: 'index,follow',
  }
}

export function merchantJoinSeoMeta(locale: SupportedLocale = DEFAULT_LOCALE): SeoMetaInput {
  const path = withLocalePath(locale, '/merchant/join')
  return {
    title: 'Join Ghana Chinatown as a Merchant | Open Your Store',
    description: 'Apply to become a merchant on Ghana Chinatown and showcase your products to local and Chinese customers in Ghana.',
    canonical: canonicalURL(path),
    alternates: localizedAlternates(path),
    robots: 'index,follow',
    type: 'website',
    image: DEFAULT_IMAGE,
  }
}

export function homeSeoMeta(locale: SupportedLocale = DEFAULT_LOCALE): SeoMetaInput {
  const path = withLocalePath(locale, '/')
  return {
    title: 'Ghana Chinatown | Ghana Chinese Marketplace',
    description: 'Discover products, shops, and marketplace services on Ghana Chinatown.',
    canonical: canonicalURL(path),
    alternates: localizedAlternates(path),
    robots: 'index,follow',
    type: 'website',
    image: DEFAULT_IMAGE,
    jsonLd: [
      {
        '@context': 'https://schema.org',
        '@type': 'WebSite',
        name: SITE_NAME,
        url: canonicalURL(path),
        potentialAction: {
          '@type': 'SearchAction',
          target: `${SITE_ORIGIN}${withLocalePath(locale, '/mall')}?keyword={search_term_string}`,
          'query-input': 'required name=search_term_string',
        },
      },
      {
        '@context': 'https://schema.org',
        '@type': 'Organization',
        name: SITE_NAME,
        url: canonicalURL(path),
      },
    ],
  }
}

export function mallSeoMeta(source: MallSeoSource = 'all', locale: SupportedLocale = DEFAULT_LOCALE): SeoMetaInput {
  const path = source === 'all' ? withLocalePath(locale, '/mall') : withLocalePath(locale, `/mall?source=${source}`)
  switch (source) {
    case 't6':
      return {
        title: 'T6 Mall Products | Ghana Chinatown',
        description: 'Browse official T6 Mall products on Ghana Chinatown.',
        canonical: canonicalURL(path),
        alternates: localizedAlternates(path),
        robots: 'index,follow',
        type: 'website',
        image: DEFAULT_IMAGE,
      }
    case 'merchant':
      return {
        title: 'Merchant Products | Ghana Chinatown',
        description: 'Browse products from public merchants on Ghana Chinatown.',
        canonical: canonicalURL(path),
        alternates: localizedAlternates(path),
        robots: 'index,follow',
        type: 'website',
        image: DEFAULT_IMAGE,
      }
    default:
      return {
        title: 'All Products | Ghana Chinatown',
        description: 'Browse all public products from T6 Mall and merchants on Ghana Chinatown.',
        canonical: canonicalURL(path),
        alternates: localizedAlternates(path),
        robots: 'index,follow',
        type: 'website',
        image: DEFAULT_IMAGE,
      }
  }
}

export function applySeoMeta(input: SeoMetaInput) {
  if (typeof document === 'undefined') {
    return
  }

  const description = input.description?.trim() || DEFAULT_DESCRIPTION
  const canonical = input.canonical || canonicalURL(input.path || '/')
  const robots = input.robots || 'index,follow'
  const image = absoluteURL(input.image)
  const type = input.type || 'website'

  document.title = input.title
  upsertMeta('name', 'description', description)
  upsertMeta('name', 'robots', robots)
  upsertLink('canonical', canonical)
  upsertMeta('property', 'og:title', input.title)
  upsertMeta('property', 'og:description', description)
  upsertMeta('property', 'og:type', type)
  upsertMeta('property', 'og:url', canonical)
  upsertMeta('property', 'og:image', image)
  upsertMeta('property', 'og:site_name', SITE_NAME)
  upsertMeta('name', 'twitter:card', 'summary_large_image')
  upsertMeta('name', 'twitter:title', input.title)
  upsertMeta('name', 'twitter:description', description)
  upsertMeta('name', 'twitter:image', image)
  setHreflang(robots.startsWith('noindex') ? null : input.alternates ?? localizedAlternates(pathFromCanonical(canonical)))
  setJsonLd(input.jsonLd || [])
}

export function useSeoMeta(input: MaybeRef<SeoMetaInput>) {
  const stop = watchEffect(() => {
    applySeoMeta(unref(input))
  })
  onBeforeUnmount(stop)
}

function normalizePath(path: string) {
  const cleanPath = path.split('?')[0]?.split('#')[0] || '/'
  if (cleanPath.length > 1 && cleanPath.endsWith('/')) {
    return cleanPath.slice(0, -1)
  }
  return cleanPath || '/'
}

function normalizeCanonicalPath(path: string) {
  const locale = localeFromPath(path) || DEFAULT_LOCALE
  const localizedPath = withLocalePath(locale, path)
  const cleanPath = normalizePath(localizedPath)
  const storefrontPath = normalizePath(stripLocalePrefix(cleanPath))
  if (storefrontPath !== '/mall') {
    return cleanPath
  }

  const source = mallSeoSourceFromPath(path)
  if (source === 't6' || source === 'merchant') {
    return withLocalePath(locale, `/mall?source=${source}`)
  }
  return withLocalePath(locale, '/mall')
}

function mallSeoSourceFromPath(path: string): MallSeoSource {
  const query = path.split('?')[1]?.split('#')[0] || ''
  const source = new URLSearchParams(query).get('source')?.trim().toLowerCase()
  return source === 't6' || source === 'merchant' ? source : 'all'
}

function upsertMeta(attribute: 'name' | 'property', key: string, content: string) {
  const selector = `meta[${attribute}="${key}"]`
  let element = document.head.querySelector<HTMLMetaElement>(selector)
  if (!element) {
    element = document.createElement('meta')
    element.setAttribute(attribute, key)
    document.head.appendChild(element)
  }
  element.dataset.seo = 'managed'
  element.setAttribute('content', content)
}

function upsertLink(rel: string, href: string) {
  let element = document.head.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`)
  if (!element) {
    element = document.createElement('link')
    element.setAttribute('rel', rel)
    document.head.appendChild(element)
  }
  element.dataset.seo = 'managed'
  element.setAttribute('href', href)
}

function setHreflang(alternates?: SeoAlternateLinks | null) {
  document.head.querySelectorAll('link[data-seo-alternate="true"]').forEach((element) => element.remove())
  if (!alternates) {
    return
  }

  for (const [hreflang, href] of [
    ['en', alternates.en],
    ['zh', alternates.zh],
    ['x-default', alternates.xDefault],
  ] as const) {
    const element = document.createElement('link')
    element.rel = 'alternate'
    element.hreflang = hreflang
    element.href = href
    element.dataset.seoAlternate = 'true'
    document.head.appendChild(element)
  }
}

function localizedAlternates(path = '/'): SeoAlternateLinks {
  const canonicalPath = normalizeCanonicalPath(path)
  const storefrontPath = stripLocalePrefix(canonicalPath)
  const urls = Object.fromEntries(
    SUPPORTED_LOCALES.map((locale) => [locale, canonicalURL(withLocalePath(locale, storefrontPath))]),
  ) as Record<SupportedLocale, string>
  return {
    en: urls.en,
    zh: urls.zh,
    xDefault: urls.en,
  }
}

function pathFromCanonical(canonical: string) {
  try {
    const url = new URL(canonical)
    return `${url.pathname}${url.search}`
  } catch {
    return canonical
  }
}

function setJsonLd(items: unknown[]) {
  document.head.querySelectorAll('script[data-seo-jsonld="true"]').forEach((element) => element.remove())
  items.forEach((item) => {
    const script = document.createElement('script')
    script.type = 'application/ld+json'
    script.dataset.seoJsonld = 'true'
    script.textContent = JSON.stringify(item)
    document.head.appendChild(script)
  })
}
