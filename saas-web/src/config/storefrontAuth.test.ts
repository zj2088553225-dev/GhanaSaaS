import { describe, expect, it } from 'vitest'
import {
  isCustomerAuthRouteUnavailable,
  isPasswordResetRouteUnavailable,
} from './storefrontAuth'

describe('storefront customer authentication flag', () => {
  it.each([
    'customer-login',
    'customer-register',
    'forgot-password',
    'reset-password',
    'customer-account',
  ])('blocks %s when customer authentication is disabled', (routeName) => {
    expect(isCustomerAuthRouteUnavailable(routeName, false)).toBe(true)
    expect(isCustomerAuthRouteUnavailable(routeName, true)).toBe(false)
  })

  it('does not affect administrator or public routes', () => {
    expect(isCustomerAuthRouteUnavailable('admin-login', false)).toBe(false)
    expect(isCustomerAuthRouteUnavailable('home', false)).toBe(false)
  })

  it('blocks reset routes unless both authentication and reset delivery are enabled', () => {
    expect(isPasswordResetRouteUnavailable('forgot-password', true, false)).toBe(true)
    expect(isPasswordResetRouteUnavailable('reset-password', false, true)).toBe(true)
    expect(isPasswordResetRouteUnavailable('forgot-password', true, true)).toBe(false)
    expect(isPasswordResetRouteUnavailable('customer-login', true, false)).toBe(false)
  })
})
