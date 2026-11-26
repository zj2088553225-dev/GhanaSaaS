export const DEFAULT_CURRENCY = 'GHS'

export function formatMoney(amountMinor: number, currency = DEFAULT_CURRENCY) {
  return `${currency} ${(amountMinor / 100).toFixed(2)}`
}
