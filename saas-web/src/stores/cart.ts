import { defineStore } from 'pinia'
import { isPositiveInteger, mergeCartItems, normalizeCartLine, type CartLine } from '../utils/cartItems'

export type { CartLine } from '../utils/cartItems'

const CART_STORAGE_KEY = 'saas_customer_cart'

export interface CartState {
  lines: CartLine[]
}

function readStoredCart(): CartState {
  try {
    const raw = localStorage.getItem(CART_STORAGE_KEY)
    if (!raw) {
      return { lines: [] }
    }

    const parsed = JSON.parse(raw) as Partial<CartState>
    const lines = Array.isArray(parsed.lines) ? parsed.lines.map(normalizeCartLine).filter((line): line is CartLine => Boolean(line)) : []

    return { lines }
  } catch {
    localStorage.removeItem(CART_STORAGE_KEY)
    return { lines: [] }
  }
}

function persistCart(lines: CartLine[]) {
  if (!lines.length) {
    localStorage.removeItem(CART_STORAGE_KEY)
    return
  }

  localStorage.setItem(
    CART_STORAGE_KEY,
    JSON.stringify({
      lines: lines.map((line) => ({
        sku_id: line.sku_id,
        product_id: line.product_id,
        quantity: line.quantity,
      })),
    }),
  )
}

export const useCartStore = defineStore('cart', {
  state: (): CartState => readStoredCart(),
  getters: {
    totalQuantity: (state) => state.lines.reduce((total, line) => total + line.quantity, 0),
  },
  actions: {
    addItem(product_id: number, sku_id: number, quantity: number) {
      this.addItems([{ product_id, sku_id, quantity }])
    },
    addItems(items: { product_id: number; sku_id: number; quantity: number }[]) {
      this.lines = mergeCartItems(this.lines, items)
      persistCart(this.lines)
    },
    setQuantity(sku_id: number, quantity: number) {
      const index = this.lines.findIndex((line) => line.sku_id === sku_id)
      if (index === -1) {
        return
      }

      if (!isPositiveInteger(quantity)) {
        this.lines.splice(index, 1)
      } else {
        this.lines[index].quantity = quantity
      }

      persistCart(this.lines)
    },
    removeItem(sku_id: number) {
      this.lines = this.lines.filter((line) => line.sku_id !== sku_id)
      persistCart(this.lines)
    },
    clearCart() {
      this.lines = []
      persistCart(this.lines)
    },
    quantityForSku(sku_id: number) {
      return this.lines.find((line) => line.sku_id === sku_id)?.quantity ?? 0
    },
  },
})
