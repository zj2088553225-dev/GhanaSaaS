// @vitest-environment jsdom

import { mount, shallowMount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import en from '../locales/en'
import { APIRequestError } from '../utils/request'

const apiMocks = vi.hoisted(() => ({
  createAnonymousReservation: vi.fn(),
  getMyAnonymousReservations: vi.fn(),
  cancelAnonymousReservation: vi.fn(),
}))

vi.mock('../api/reservations', () => ({
  ...apiMocks,
}))

const reservation = {
  reservation_code: 'GC-AbCdEfGhIjKlMnOpQrStUvWx',
  identity_type: 'anonymous',
  status: 'pending',
  quantity: 2,
  created_at: '2026-07-17T12:00:00Z',
  expires_at: '2026-07-18T12:00:00Z',
  product: {
    id: 7,
    slug: 'anonymous-product',
    title: 'Anonymous product',
    main_image_url: '/uploads/product.jpg',
  },
} as const

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  fallbackLocale: 'en',
  messages: { en },
})

function testRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/:locale(en|zh)/reservations', component: { template: '<div />' } },
      { path: '/:locale(en|zh)/reservations/mine', component: { template: '<div />' } },
    ],
  })
}

const turnstileStub = {
  props: ['action'],
  emits: ['token'],
  template: '<button type="button" data-testid="turnstile-stub" @click="$emit(\'token\', \'opaque-turnstile-token\')">{{ action }}</button>',
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

describe('anonymous reservation storefront', () => {
  beforeAll(() => {
    Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() })
    Object.defineProperty(globalThis, 'sessionStorage', { configurable: true, value: memoryStorage() })
    vi.stubEnv('VITE_RESERVATION_MODE', 'anonymous')
  })

  beforeEach(() => {
    apiMocks.createAnonymousReservation.mockReset()
    apiMocks.getMyAnonymousReservations.mockReset()
    apiMocks.cancelAnonymousReservation.mockReset()
  })

  afterAll(() => {
    vi.unstubAllEnvs()
  })

  it('does not render phone/OTP controls and creates only after a reservation_create Turnstile token', async () => {
    apiMocks.createAnonymousReservation.mockResolvedValue(reservation)
    const { default: AnonymousReservationDialog } = await import('../components/storefront/AnonymousReservationDialog.vue')
    const router = testRouter()
    await router.push('/en/reservations')
    await router.isReady()
    const wrapper = mount(AnonymousReservationDialog, {
      props: { open: true, productId: 7, productTitle: 'Anonymous product' },
      global: {
        plugins: [i18n, router],
        stubs: { Teleport: true, CloudflareTurnstile: turnstileStub },
      },
    })

    expect(wrapper.find('input[type="tel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="reservation-send-otp"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="turnstile-stub"]').text()).toBe('reservation_create')
    expect(wrapper.get('[data-testid="anonymous-reservation-submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="turnstile-stub"]').trigger('click')
    await wrapper.get('[data-testid="anonymous-reservation-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createAnonymousReservation).toHaveBeenCalledWith(7, {
      quantity: 1,
      note: undefined,
      turnstile_token: 'opaque-turnstile-token',
    })
    expect(wrapper.get('[data-testid="anonymous-reservation-success"]').text()).toContain(reservation.reservation_code)
    expect(wrapper.text()).toContain('Reservation created')
  })

  it('shows the existing reservation for a duplicate and a bounded retry message for 429', async () => {
    const { default: AnonymousReservationDialog } = await import('../components/storefront/AnonymousReservationDialog.vue')
    const router = testRouter()
    await router.push('/en/reservations')
    await router.isReady()

    apiMocks.createAnonymousReservation.mockRejectedValueOnce(new APIRequestError('duplicate', {
      status: 409,
      code: 'RESERVATION_ALREADY_EXISTS',
      data: reservation,
    }))
    const duplicate = mount(AnonymousReservationDialog, {
      props: { open: true, productId: 7, productTitle: 'Anonymous product' },
      global: {
        plugins: [i18n, router],
        stubs: { Teleport: true, CloudflareTurnstile: turnstileStub },
      },
    })
    await duplicate.get('[data-testid="turnstile-stub"]').trigger('click')
    await duplicate.get('[data-testid="anonymous-reservation-submit"]').trigger('click')
    await flushPromises()
    expect(duplicate.text()).toContain('You already reserved this product')
    expect(duplicate.text()).toContain(reservation.reservation_code)

    apiMocks.createAnonymousReservation.mockRejectedValueOnce(new APIRequestError('limited', {
      status: 429,
      code: 'BROWSER_RATE_LIMITED',
      retryAfterSeconds: 3600,
    }))
    const limited = mount(AnonymousReservationDialog, {
      props: { open: true, productId: 8, productTitle: 'Limited product' },
      global: {
        plugins: [i18n, router],
        stubs: { Teleport: true, CloudflareTurnstile: turnstileStub },
      },
    })
    await limited.get('[data-testid="turnstile-stub"]').trigger('click')
    await limited.get('[data-testid="anonymous-reservation-submit"]').trigger('click')
    await flushPromises()
    expect(limited.get('[role="alert"]').text()).toContain('Please wait before trying again')
    expect(apiMocks.createAnonymousReservation).toHaveBeenCalledTimes(2)
  })

  it('loads My Reservations through the cookie-backed endpoint', async () => {
    apiMocks.getMyAnonymousReservations.mockResolvedValue([reservation])
    const { default: MyAnonymousReservations } = await import('../views/reservations/MyAnonymousReservations.vue')
    const router = testRouter()
    await router.push('/en/reservations/mine')
    await router.isReady()
    const wrapper = shallowMount(MyAnonymousReservations, {
      global: {
        plugins: [i18n, router],
        stubs: {
          StorefrontHeader: true,
          ProductImage: true,
          RouterLink: { template: '<a><slot /></a>' },
        },
      },
    })
    await flushPromises()
    expect(apiMocks.getMyAnonymousReservations).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="my-reservations-list"]').text()).toContain(reservation.reservation_code)
    expect(wrapper.text()).toContain('An IP address alone can never be used')
  })

  it('keeps anonymous reservation routes available in English and Chinese', async () => {
    const { default: router } = await import('../router')
    await router.push('/en/reservations/mine')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/en/reservations/mine')
    await router.push('/zh/reservations/mine')
    expect(router.currentRoute.value.path).toBe('/zh/reservations/mine')
  })
})
