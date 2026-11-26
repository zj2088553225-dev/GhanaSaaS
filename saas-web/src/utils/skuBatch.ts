export interface SKUBatchValues {
  retail_price?: number | null
  original_price?: number | null
  sellable_stock?: number | null
}

export interface SKUPriceStockRow {
  erp_sku_code: string
  retail_price: number | null
  original_price: number | null
  sellable_stock: number | null
}

export function hasSkuBatchValues(values: SKUBatchValues) {
  return values.retail_price != null || values.original_price != null || values.sellable_stock != null
}

export function isValidBatchStock(value: number | null | undefined) {
  return value == null || (Number.isInteger(value) && value >= 0)
}

export function applySkuBatchToRows<T extends SKUPriceStockRow>(rows: T[], values: SKUBatchValues): T[] {
  const hasRetailPrice = values.retail_price != null
  const hasOriginalPrice = values.original_price != null
  const hasStock = values.sellable_stock != null

  return rows.map((row) => ({
    ...row,
    retail_price: hasRetailPrice ? values.retail_price! : row.retail_price,
    original_price: hasOriginalPrice ? values.original_price! : row.original_price,
    sellable_stock: hasStock ? values.sellable_stock! : row.sellable_stock,
  }))
}
