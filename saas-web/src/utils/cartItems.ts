export interface CartLine {
  sku_id: number
  product_id: number
  quantity: number
}

export function isPositiveInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
}

export function normalizeCartLine(value: unknown): CartLine | null {
  if (!value || typeof value !== 'object') {
    return null
  }

  const candidate = value as Partial<CartLine>
  if (!isPositiveInteger(candidate.sku_id) || !isPositiveInteger(candidate.product_id) || !isPositiveInteger(candidate.quantity)) {
    return null
  }

  return {
    sku_id: candidate.sku_id,
    product_id: candidate.product_id,
    quantity: candidate.quantity,
  }
}

export function mergeCartItems(lines: readonly CartLine[], items: readonly Partial<CartLine>[]): CartLine[] {
  const next = lines.map((line) => ({ ...line }))

  for (const item of items) {
    const normalized = normalizeCartLine(item)
    if (!normalized) {
      continue
    }

    const existing = next.find((line) => line.sku_id === normalized.sku_id)
    if (existing) {
      existing.quantity += normalized.quantity
    } else {
      next.push(normalized)
    }
  }

  return next
}
