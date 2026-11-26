// @vitest-environment jsdom

import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key: string) {
      return values.get(key) ?? null
    },
    key(index: number) {
      return Array.from(values.keys())[index] ?? null
    },
    removeItem(key: string) {
      values.delete(key)
    },
    setItem(key: string, value: string) {
      values.set(key, String(value))
    },
  }
}

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  fallbackLocale: 'en',
  missingWarn: false,
  fallbackWarn: false,
  messages: { en: {} },
})

describe('reservation feature disabled', () => {
  beforeAll(() => {
    Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() })
    Object.defineProperty(globalThis, 'sessionStorage', { configurable: true, value: memoryStorage() })
    vi.stubEnv('VITE_RESERVATION_MODE', 'disabled')
  })

  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    sessionStorage.clear()
    document.body.innerHTML = ''
  })

  afterAll(() => {
    vi.unstubAllEnvs()
  })

  it('defaults reservation mode to disabled and accepts only supported modes', async () => {
    const { parseReservationMode, parseStorefrontFeatureFlag, reservationEnabled, reservationMode } = await import('./storefront')

    expect(reservationEnabled).toBe(false)
    expect(reservationMode).toBe('disabled')
    expect(parseReservationMode(undefined)).toBe('disabled')
    expect(parseReservationMode('false')).toBe('disabled')
    expect(parseReservationMode('anonymous')).toBe('anonymous')
    expect(parseReservationMode('OTP')).toBe('otp')
    expect(parseStorefrontFeatureFlag(undefined)).toBe(false)
    expect(parseStorefrontFeatureFlag('false')).toBe(false)
    expect(parseStorefrontFeatureFlag('true')).toBe(true)
  })

  it('hides storefront and Reservation Products navigation', async () => {
    const [{ default: StorefrontHeader }, { default: AdminLayout }, { useAuthStore }] = await Promise.all([
      import('../components/storefront/StorefrontHeader.vue'),
      import('../layouts/AdminLayout.vue'),
      import('../stores/auth'),
    ])
    const pinia = createPinia()
    setActivePinia(pinia)
    const authStore = useAuthStore()
    authStore.$patch({
      token: 'test-token',
      user: { id: 1, role: 'super_admin', phone: 'test', status: 'active' },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/:locale(en|zh)', component: { template: '<div />' } },
        { path: '/gc-control-7x/:pathMatch(.*)*', component: { template: '<div />' } },
      ],
    })
    await router.push('/en')
    await router.isReady()

    const storefront = shallowMount(StorefrontHeader, {
      global: { plugins: [pinia, router, i18n] },
    })
    expect(storefront.find('[data-testid="reservation-navigation"]').exists()).toBe(false)

    await router.push('/gc-control-7x/global-dashboard')
    const admin = shallowMount(AdminLayout, {
      global: {
        plugins: [pinia, router],
        stubs: {
          ElAside: { template: '<aside><slot /></aside>' },
          ElContainer: { template: '<div><slot /></div>' },
          ElHeader: { template: '<header><slot /></header>' },
          ElMain: { template: '<main><slot /></main>' },
          ElMenu: { template: '<nav><slot /></nav>' },
          ElMenuItem: { template: '<span><slot /></span>' },
          RouterView: true,
        },
      },
    })
    expect(admin.text()).not.toContain('预约商品')
    expect(admin.text()).not.toContain('预约记录')
  })

  it('redirects direct reservation routes to the localized home page', async () => {
    const { default: router } = await import('../router')

    await router.push('/en/reservations')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/en')

    await router.push('/zh/reservations/example')
    expect(router.currentRoute.value.path).toBe('/zh')
  })

  it('hides reservation and send-code buttons without requesting reservation APIs', async () => {
    const [{ default: request }, { default: ReservationProductDetail }, { default: ReservationVerificationDialog }] = await Promise.all([
      import('../utils/request'),
      import('../views/reservations/ReservationProductDetail.vue'),
      import('../components/storefront/ReservationVerificationDialog.vue'),
    ])
    const getSpy = vi.spyOn(request, 'get')
    const postSpy = vi.spyOn(request, 'post')
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/:locale(en|zh)/reservations/:slug', component: { template: '<div />' } },
      ],
    })
    await router.push('/en/reservations/example')
    await router.isReady()

    const detail = shallowMount(ReservationProductDetail, {
      global: { plugins: [router, i18n] },
    })
    await flushPromises()
    expect(detail.find('[data-testid="reservation-interest"]').exists()).toBe(false)
    expect(detail.find('[data-testid="reservation-submit"]').exists()).toBe(false)

    const dialog = shallowMount(ReservationVerificationDialog, {
      props: { open: true, productId: 1, productTitle: 'Example' },
      global: {
        plugins: [i18n],
        stubs: { Teleport: true },
      },
    })
    await flushPromises()
    expect(dialog.find('[data-testid="reservation-send-otp"]').exists()).toBe(false)
    expect(getSpy).not.toHaveBeenCalled()
    expect(postSpy).not.toHaveBeenCalled()
  })

  it('blocks OTP calls in the API client before Axios is invoked', async () => {
    const [{ default: request }, { requestReservationOTP }] = await Promise.all([
      import('../utils/request'),
      import('../api/reservations'),
    ])
    const postSpy = vi.spyOn(request, 'post')

    await expect(requestReservationOTP(1, {
      visitor_id: '11111111-1111-4111-8111-111111111111',
      idempotency_key: '22222222-2222-4222-8222-222222222222',
      contact_type: 'sms',
      phone_country: 'GH',
      phone_number: '0241234567',
      turnstile_token: '',
      source_type: 'reservation_dialog',
    })).rejects.toThrow('Reservation feature is disabled')
    expect(postSpy).not.toHaveBeenCalled()
  })
})
