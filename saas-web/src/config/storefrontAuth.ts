import { parseStorefrontFeatureFlag } from './storefront'

const storefrontEnvironment = typeof import.meta.env === 'object' ? import.meta.env : undefined

export const storefrontAuthEnabled = parseStorefrontFeatureFlag(
  storefrontEnvironment?.VITE_STOREFRONT_AUTH_ENABLED,
)

export const customerPasswordResetEnabled = parseStorefrontFeatureFlag(
  storefrontEnvironment?.VITE_CUSTOMER_PASSWORD_RESET_ENABLED,
)

export const customerAuthRouteNames = new Set([
  'customer-login',
  'customer-register',
  'forgot-password',
  'reset-password',
  'customer-account',
])

export function isCustomerAuthRouteUnavailable(
  routeName: unknown,
  enabled = storefrontAuthEnabled,
) {
  return !enabled && customerAuthRouteNames.has(String(routeName || ''))
}

export function isPasswordResetRouteUnavailable(
  routeName: unknown,
  authEnabled = storefrontAuthEnabled,
  passwordResetEnabled = customerPasswordResetEnabled,
) {
  const normalized = String(routeName || '')
  return (normalized === 'forgot-password' || normalized === 'reset-password') &&
    (!authEnabled || !passwordResetEnabled)
}
