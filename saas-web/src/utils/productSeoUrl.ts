import { withLocalePath, type SupportedLocale } from './localePath.ts'

export interface ProductSeoPathInput {
  id: number | string
  name?: string
  title?: string
}

export function buildProductSlug(name: string): string {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/&/g, ' and ')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 80)
    .replace(/^-|-$/g, '')

  return slug || 'product'
}

export function buildProductSeoPath(product: ProductSeoPathInput, locale?: SupportedLocale): string {
  const id = String(product.id).trim()
  const name = product.name?.trim() || product.title?.trim() || ''
  const path = `/products/${buildProductSlug(name)}-${id}`
  return locale ? withLocalePath(locale, path) : path
}

export function parseProductIdFromSeoParam(param: string): number {
  const match = param.trim().match(/(?:^|-)(\d+)$/)
  if (!match) {
    return 0
  }
  const id = Number(match[1])
  return Number.isInteger(id) && id > 0 ? id : 0
}
