export type MallSource = 'all' | 't6' | 'merchant'

export const mallSourceOptions: MallSource[] = ['all', 't6', 'merchant']

export function normalizeMallSource(value: unknown): MallSource {
  const source = queryStringValue(value).trim().toLowerCase()
  return source === 't6' || source === 'merchant' ? source : 'all'
}

export function mallSourceQueryValue(source: MallSource): MallSource | undefined {
  return source === 'all' ? undefined : source
}

export function productSourceParam(value: unknown): MallSource | undefined {
  const source = queryStringValue(value).trim().toLowerCase()
  if (source === 't6' || source === 'merchant') {
    return source
  }
  if (source === 'featured') {
    return 't6'
  }
  return undefined
}

export function productSourceLinkQuery(source: unknown): Record<string, string> {
  const normalized = productSourceParam(source)
  return normalized ? { source: normalized } : {}
}

export function mallSourceRouteQuery(
  current: {
    keyword?: string
    platformCategoryId?: number
    sort?: string
  },
  source: MallSource,
): Record<string, string> {
  const query: Record<string, string> = {}
  if (current.keyword) {
    query.keyword = current.keyword
  }
  if (current.platformCategoryId) {
    query.platform_category_id = String(current.platformCategoryId)
  }
  if (current.sort && current.sort !== 'comprehensive') {
    query.sort = current.sort
  }
  const sourceValue = mallSourceQueryValue(source)
  if (sourceValue) {
    query.source = sourceValue
  }
  return query
}

function queryStringValue(value: unknown) {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}
