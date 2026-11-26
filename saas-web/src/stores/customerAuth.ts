import { defineStore } from 'pinia'

const CUSTOMER_TOKEN_KEY = 'saas_customer_token'
const CUSTOMER_PROFILE_KEY = 'saas_customer_profile'

export interface CustomerProfile {
  id: number
  email: string
  phone?: string
  created_at: string
}

export interface CustomerAuthState {
  token: string
  customer: CustomerProfile | null
}

export interface CustomerAuthPayload {
  token: string
  user: CustomerProfile
}

function readStoredCustomerAuth(): CustomerAuthState {
  const fallback: CustomerAuthState = { token: '', customer: null }

  try {
    const token = localStorage.getItem(CUSTOMER_TOKEN_KEY) || ''
    const rawCustomer = localStorage.getItem(CUSTOMER_PROFILE_KEY)
    if (!token || !rawCustomer) {
      return fallback
    }

    return {
      token,
      customer: JSON.parse(rawCustomer),
    }
  } catch {
    clearStoredCustomerAuth()
    return fallback
  }
}

function persistCustomerAuth(state: CustomerAuthState) {
  if (!state.token || !state.customer) {
    clearStoredCustomerAuth()
    return
  }

  localStorage.setItem(CUSTOMER_TOKEN_KEY, state.token)
  localStorage.setItem(CUSTOMER_PROFILE_KEY, JSON.stringify(state.customer))
}

export function getStoredCustomerToken() {
  return readStoredCustomerAuth().token
}

export function clearStoredCustomerAuth() {
  localStorage.removeItem(CUSTOMER_TOKEN_KEY)
  localStorage.removeItem(CUSTOMER_PROFILE_KEY)
}

export const useCustomerAuthStore = defineStore('customerAuth', {
  state: (): CustomerAuthState => readStoredCustomerAuth(),
  getters: {
    isAuthenticated: (state) => Boolean(state.token),
  },
  actions: {
    setAuth(payload: CustomerAuthPayload) {
      this.token = payload.token
      this.customer = payload.user
      persistCustomerAuth({ token: this.token, customer: this.customer })
    },
    clearAuth() {
      this.token = ''
      this.customer = null
      clearStoredCustomerAuth()
    },
    logout() {
      this.clearAuth()
    },
  },
})
