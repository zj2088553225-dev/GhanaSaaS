// @vitest-environment jsdom

import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import StoreAuthStatus from '../../components/StoreAuthStatus.vue'
import en from '../../locales/en'
import { useCustomerAuthStore, type CustomerProfile } from '../../stores/customerAuth'
import Account from '../Account.vue'
import CustomerLogin from './CustomerLogin.vue'
import Register from './Register.vue'

const authAPIMocks = vi.hoisted(() => ({
  loginCustomer: vi.fn(),
  registerCustomer: vi.fn(),
  getCurrentCustomer: vi.fn(),
  logoutCustomer: vi.fn(),
}))

vi.mock('../../api/auth', () => ({
  ...authAPIMocks,
}))

vi.mock('../../components/storefront/StorefrontHeader.vue', () => ({
  default: { template: '<header />' },
}))

const profile: CustomerProfile = {
  id: 7,
  email: 'buyer@example.com',
  created_at: '2026-07-30T10:00:00Z',
}

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => {
      values.delete(key)
    },
    setItem: (key, value) => values.set(key, String(value)),
  }
}

function testI18n() {
  return createI18n({
    legacy: false,
    locale: 'en',
    fallbackLocale: 'en',
    messages: { en },
  })
}

function testRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/en/login', component: CustomerLogin },
      { path: '/en/register', component: Register },
      { path: '/en/forgot-password', component: { template: '<div />' } },
      { path: '/en/account', component: Account },
    ],
  })
}

function authPageGlobal(pinia: ReturnType<typeof createPinia>, router: ReturnType<typeof testRouter>) {
  return {
    plugins: [pinia, router, testI18n()],
    stubs: { StorefrontHeader: true },
  }
}

beforeEach(() => {
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() })
  authAPIMocks.loginCustomer.mockReset()
  authAPIMocks.registerCustomer.mockReset()
  authAPIMocks.getCurrentCustomer.mockReset()
  authAPIMocks.logoutCustomer.mockReset()
})

describe('customer authentication pages', () => {
  it('renders email-only registration and validates weak passwords', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = testRouter()
    await router.push('/en/register')
    await router.isReady()
    const wrapper = mount(Register, {
      global: authPageGlobal(pinia, router),
    })

    expect(wrapper.find('input[type="email"]').exists()).toBe(true)
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(2)
    expect(wrapper.find('input[type="tel"]').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/verification code|SMS/i)

    await wrapper.get('input[type="email"]').setValue('buyer@example.com')
    await wrapper.findAll('input[type="password"]')[0].setValue('short')
    await wrapper.findAll('input[type="password"]')[1].setValue('short')
    await wrapper.get('[data-testid="register-form"]').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toContain('at least 8 characters')
    expect(authAPIMocks.registerCustomer).not.toHaveBeenCalled()
  })

  it('registers without creating a login session', async () => {
    authAPIMocks.registerCustomer.mockResolvedValue({ user: profile })
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = testRouter()
    await router.push('/en/register')
    await router.isReady()
    const wrapper = mount(Register, {
      global: authPageGlobal(pinia, router),
    })

    await wrapper.get('input[type="email"]').setValue(' Buyer@Example.COM ')
    await wrapper.findAll('input[type="password"]')[0].setValue('strong-password')
    await wrapper.findAll('input[type="password"]')[1].setValue('strong-password')
    await wrapper.get('[data-testid="register-form"]').trigger('submit')
    await flushPromises()

    expect(authAPIMocks.registerCustomer).toHaveBeenCalledWith({
      email: 'buyer@example.com',
      password: 'strong-password',
    })
    expect(wrapper.get('[role="status"]').text()).toContain('Please log in')
    expect(useCustomerAuthStore().isAuthenticated).toBe(false)
  })

  it('shows login errors and provides the forgot-password link', async () => {
    authAPIMocks.loginCustomer.mockRejectedValue(new Error('Invalid email or password'))
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = testRouter()
    await router.push('/en/login')
    await router.isReady()
    const wrapper = mount(CustomerLogin, {
      props: { passwordResetEnabled: true },
      global: authPageGlobal(pinia, router),
    })

    expect(wrapper.get('a[href="/en/forgot-password"]').text()).toContain('Forgot password')
    await wrapper.get('input[type="email"]').setValue('buyer@example.com')
    await wrapper.get('input[type="password"]').setValue('wrong-password')
    await wrapper.get('[data-testid="login-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toBe('Invalid email or password')
    expect(useCustomerAuthStore().isAuthenticated).toBe(false)
  })

  it('hides forgot password when delivery is disabled', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = testRouter()
    await router.push('/en/login')
    await router.isReady()
    const wrapper = mount(CustomerLogin, {
      props: { passwordResetEnabled: false },
      global: authPageGlobal(pinia, router),
    })

    expect(wrapper.find('a[href="/en/forgot-password"]').exists()).toBe(false)
  })

  it('switches navigation from Login to Account after login', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = testRouter()
    await router.push('/en/login')
    await router.isReady()
    const wrapper = shallowMount(StoreAuthStatus, {
      props: { enabled: true },
      global: {
        plugins: [pinia, router, testI18n()],
        stubs: { RouterLink: false },
      },
    })
    expect(wrapper.text()).toBe('Login')

    useCustomerAuthStore().setAuth({ token: 'customer-jwt', user: profile })
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toBe('Account')
    expect(wrapper.get('a').attributes('href')).toBe('/en/account')
  })

  it('does not render customer authentication navigation when disabled', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = testRouter()
    await router.push('/en/login')
    await router.isReady()
    const wrapper = shallowMount(StoreAuthStatus, {
      props: { enabled: false },
      global: {
        plugins: [pinia, router, testI18n()],
        stubs: { RouterLink: false },
      },
    })

    expect(wrapper.find('[data-testid="store-auth-navigation"]').exists()).toBe(false)
  })

  it('clears customer login state when logging out from Account', async () => {
    authAPIMocks.getCurrentCustomer.mockResolvedValue(profile)
    authAPIMocks.logoutCustomer.mockResolvedValue(undefined)
    const pinia = createPinia()
    setActivePinia(pinia)
    const authStore = useCustomerAuthStore()
    authStore.setAuth({ token: 'customer-jwt', user: profile })
    const router = testRouter()
    await router.push('/en/account')
    await router.isReady()
    const wrapper = mount(Account, {
      global: authPageGlobal(pinia, router),
    })
    await flushPromises()

    await wrapper.get('[data-testid="account-logout"]').trigger('click')
    await flushPromises()
    expect(authAPIMocks.logoutCustomer).toHaveBeenCalledOnce()
    expect(authStore.isAuthenticated).toBe(false)
    expect(router.currentRoute.value.path).toBe('/en/login')
  })
})
