export interface SpecDimension {
  name: string
  values: string[]
}

export type SelectedSpecs = Record<string, string>

export interface ProductSkuLike {
  id: number
  specs?: unknown
  sellable_stock?: number
  stock?: number
  is_active?: boolean
}

export interface ProductGalleryLike {
  main_image?: unknown
  mainImage?: unknown
  image_url?: unknown
  video_url?: unknown
  images?: unknown
  slider_images?: unknown
  gallery_images?: unknown
}

export interface ProductGalleryMedia {
  type: 'image' | 'video'
  url: string
}

export interface SkuImageLike {
  image_url?: unknown
  image?: unknown
}

function parseJSON(value: unknown): unknown {
  if (typeof value !== 'string') {
    return value
  }

  try {
    return JSON.parse(value) as unknown
  } catch {
    return value
  }
}

function normalizedImageList(value: unknown): string[] {
  const parsed = parseJSON(value)
  if (typeof parsed === 'string') {
    const image = parsed.trim()
    return image ? [image] : []
  }
  if (!Array.isArray(parsed)) {
    return []
  }
  return parsed
    .filter((item): item is string => typeof item === 'string')
    .map((item) => item.trim())
    .filter(Boolean)
}

export function buildProductGallery(product: ProductGalleryLike | null | undefined): string[] {
  if (!product) {
    return []
  }

  const images: string[] = []
  const seen = new Set<string>()
  const append = (value: unknown) => {
    for (const image of normalizedImageList(value)) {
      if (seen.has(image)) {
        continue
      }
      seen.add(image)
      images.push(image)
    }
  }

  append(product.main_image)
  append(product.mainImage)
  append(product.image_url)
  append(product.images)
  append(product.gallery_images)
  append(product.slider_images)
  return images
}

export function buildProductMediaGallery(product: ProductGalleryLike | null | undefined): ProductGalleryMedia[] {
  if (!product) {
    return []
  }
  const media: ProductGalleryMedia[] = []
  const videoURL = typeof product.video_url === 'string' ? product.video_url.trim() : ''
  if (videoURL) {
    media.push({ type: 'video', url: videoURL })
  }
  for (const image of buildProductGallery(product)) {
    media.push({ type: 'image', url: image })
  }
  return media
}

export function skuImageURL(sku: SkuImageLike | null | undefined): string {
  if (!sku) {
    return ''
  }
  return normalizedImageList(sku.image_url)[0] ?? normalizedImageList(sku.image)[0] ?? ''
}

export function activeImageForSku(currentImage: string, sku: SkuImageLike | null | undefined): string {
  return skuImageURL(sku) || currentImage
}

export function normalizeSpecDimensions(value: unknown): SpecDimension[] {
  const parsed = parseJSON(value)
  if (!Array.isArray(parsed)) {
    return []
  }

  return parsed
    .map((item) => {
      if (!item || typeof item !== 'object') {
        return null
      }
      const candidate = item as { name?: unknown; values?: unknown }
      const name = typeof candidate.name === 'string' ? candidate.name.trim() : ''
      const values = Array.isArray(candidate.values)
        ? Array.from(new Set(candidate.values.filter((entry): entry is string => typeof entry === 'string' && Boolean(entry.trim())).map((entry) => entry.trim())))
        : []
      return name && values.length ? { name, values } : null
    })
    .filter((item): item is SpecDimension => Boolean(item))
}

export function normalizeSkuSpecs(value: unknown): SelectedSpecs {
  const parsed = parseJSON(value)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return {}
  }

  return Object.fromEntries(
    Object.entries(parsed as Record<string, unknown>)
      .filter(([key, entry]) => Boolean(key.trim()) && typeof entry === 'string' && Boolean(entry.trim()))
      .map(([key, entry]) => [key.trim(), (entry as string).trim()]),
  )
}

export const CANONICAL_DEFAULT_SKU_NAME = 'Default SKU'

const defaultSkuNames = new Set([
  '默认规格',
  '默认sku',
  'default sku',
  'default specification',
])

export function isDefaultSkuName(name: unknown): boolean {
  return typeof name === 'string' && defaultSkuNames.has(name.trim().toLocaleLowerCase())
}

export function localizedSkuDisplayName(
  sku: { name?: string | null; specs?: unknown },
  defaultSkuName: string,
): string {
  const specText = Object.entries(normalizeSkuSpecs(sku.specs))
    .map(([key, value]) => `${key} ${value}`)
    .join(' / ')
  if (specText) {
    return specText
  }

  const name = sku.name?.trim() || ''
  return !name || isDefaultSkuName(name) ? defaultSkuName : name
}

export function deriveSpecDimensionsFromSkus(skus: ProductSkuLike[]): SpecDimension[] {
  const valuesByName = new Map<string, Set<string>>()
  for (const sku of skus) {
    const specs = normalizeSkuSpecs(sku.specs)
    for (const [name, value] of Object.entries(specs)) {
      if (!valuesByName.has(name)) {
        valuesByName.set(name, new Set())
      }
      valuesByName.get(name)?.add(value)
    }
  }

  return Array.from(valuesByName.entries()).map(([name, values]) => ({
    name,
    values: Array.from(values),
  }))
}

export function resolveSpecDimensions(explicitSpecs: unknown, skus: ProductSkuLike[]): SpecDimension[] {
  const normalized = normalizeSpecDimensions(explicitSpecs)
  return normalized.length ? normalized : deriveSpecDimensionsFromSkus(skus)
}

export function isSkuInStock(sku: ProductSkuLike | null | undefined) {
  if (!sku) {
    return false
  }
  return (sku.sellable_stock ?? sku.stock ?? 0) > 0
}

export function skuStock(sku: ProductSkuLike | null | undefined): number {
  if (!sku) {
    return 0
  }
  return sku.sellable_stock ?? sku.stock ?? 0
}

export interface SkuBatchItem {
  sku_id: number
  quantity: number
}

export function validateSkuBatchQuantity(sku: ProductSkuLike, quantity: number, cartQuantity = 0): 'ok' | 'out_of_stock' | 'exceeds_stock' {
  const stock = skuStock(sku)
  if (!sku.is_active || stock <= 0) {
    return 'out_of_stock'
  }
  if (!Number.isInteger(quantity) || quantity < 0) {
    return 'ok'
  }
  if (quantity + cartQuantity > stock) {
    return 'exceeds_stock'
  }
  return 'ok'
}
