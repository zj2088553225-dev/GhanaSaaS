// @vitest-environment jsdom

import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PopularProducts from '../components/storefront/PopularProducts.vue'
import HomeBlockRenderer from '../components/site-blocks/HomeBlockRenderer.vue'
import en from '../locales/en'
import { useAuthStore } from '../stores/auth'
import AnalyticsPopularity from './admin/analytics/AnalyticsPopularity.vue'

const apiMocks = vi.hoisted(() => ({
  getPopularProducts: vi.fn(),
  getProducts: vi.fn(),
  getProductPopularity: vi.fn(),
}))

vi.mock('../api/storefront', () => ({
  getPopularProducts: apiMocks.getPopularProducts,
  getProducts: apiMocks.getProducts,
}))

vi.mock('../api/analytics', () => ({
  getProductPopularity: apiMocks.getProductPopularity,
}))

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => values.delete(key),
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

function storefrontRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/en', component: { template: '<div />' } },
      { path: '/en/mall', component: { template: '<div />' } },
      { path: '/en/products/:productParam', component: { template: '<div />' } },
    ],
  })
}

async function mountPopularProducts() {
  const router = storefrontRouter()
  await router.push('/en')
  await router.isReady()
  return mount(PopularProducts, {
    props: {
      block: {
        type: 'product_floor',
        tracking_key: 'homepage-popular',
        limit: 10,
        columns: 5,
      },
    },
    global: {
      plugins: [router, testI18n()],
      stubs: {
        ProductImage: { props: ['alt'], template: '<div class="product-image">{{ alt }}</div>' },
        StorefrontProductLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

beforeEach(() => {
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() })
  Object.defineProperty(globalThis, 'sessionStorage', { configurable: true, value: memoryStorage() })
  apiMocks.getPopularProducts.mockReset()
  apiMocks.getProducts.mockReset()
  apiMocks.getProductPopularity.mockReset()
})

describe('storefront Popular Products', () => {
  it('renders products from the popularity API with the Popular badge but no score', async () => {
    apiMocks.getPopularProducts.mockResolvedValue([{
      id: 7,
      name: 'Phone Case',
      main_image: '',
      min_price: 100,
      max_price: 100,
      original_price: 120,
    }])
    const wrapper = await mountPopularProducts()
    await flushPromises()

    expect(wrapper.get('[data-testid="popular-products"]').text()).toContain('Popular Products')
    expect(wrapper.text()).toContain('Phone Case')
    expect(wrapper.text()).toContain('🔥 Popular')
    expect(apiMocks.getPopularProducts).toHaveBeenCalledWith({ limit: 10 })
  })

  it('shows the empty state when there are no popularity signals', async () => {
    apiMocks.getPopularProducts.mockResolvedValue([])
    const wrapper = await mountPopularProducts()
    await flushPromises()

    expect(wrapper.get('[data-testid="popular-empty"]').text()).toContain('Popular products will appear')
  })

  it('falls back to latest products when the popularity API fails', async () => {
    apiMocks.getPopularProducts.mockRejectedValue(new Error('network unavailable'))
    apiMocks.getProducts.mockResolvedValue({
      items: [{
        id: 8,
        name: 'Latest Case',
        main_image: '',
        min_price: 100,
        max_price: 100,
        original_price: 120,
      }],
      total: 1,
      page: 1,
      page_size: 10,
    })
    const wrapper = await mountPopularProducts()
    await flushPromises()

    expect(wrapper.get('[data-testid="popular-fallback"]').text()).toContain('latest available products')
    expect(wrapper.text()).toContain('Latest Case')
    expect(wrapper.find('[data-testid="popular-error"]').exists()).toBe(false)
    expect(apiMocks.getProducts).toHaveBeenCalledWith({
      sort: 'latest',
      page: 1,
      page_size: 10,
    })
  })

  it('shows an error only when popularity and fallback APIs fail, then retries', async () => {
    apiMocks.getPopularProducts
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockResolvedValueOnce([])
    apiMocks.getProducts.mockRejectedValueOnce(new Error('fallback unavailable'))
    const wrapper = await mountPopularProducts()
    await flushPromises()

    expect(wrapper.get('[data-testid="popular-error"]').text()).toContain('could not be loaded')
    await wrapper.get('[data-testid="popular-error"] button').trigger('click')
    await flushPromises()
    expect(apiMocks.getPopularProducts).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="popular-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="popular-empty"]').exists()).toBe(true)
  })

  it('uses explicit source_type instead of treating the first floor as popular', async () => {
    apiMocks.getProducts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })
    apiMocks.getPopularProducts.mockResolvedValue([])
    const router = storefrontRouter()
    await router.push('/en')
    await router.isReady()
    mount(HomeBlockRenderer, {
      props: {
        blocks: [
          { type: 'product_floor', source_type: 'latest', title: 'First latest', limit: 10 },
          { type: 'product_floor', source_type: 'popular', title: 'Second popular', limit: 10 },
        ],
      },
      global: {
        plugins: [router, testI18n()],
        stubs: {
          ProductImage: true,
          StorefrontProductLink: true,
        },
      },
    })
    await flushPromises()

    expect(apiMocks.getProducts).toHaveBeenCalledOnce()
    expect(apiMocks.getProducts).toHaveBeenCalledWith(expect.objectContaining({ sort: 'latest' }))
    expect(apiMocks.getPopularProducts).toHaveBeenCalledOnce()
  })
})

describe('admin Analytics Product Ranking', () => {
  it('renders the ranking page and keeps tenant admin requests tenant-scoped', async () => {
    apiMocks.getProductPopularity.mockResolvedValue([])
    const pinia = createPinia()
    setActivePinia(pinia)
    useAuthStore().setAuth({
      token: 'tenant-admin-token',
      user: { id: 3, tenant_id: 23, role: 'tenant_admin', phone: '233', status: 'active' },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{
        path: '/gc-control-7x/analytics/popularity',
        name: 'admin-analytics-popularity',
        component: AnalyticsPopularity,
        meta: { requiresAuth: true, roles: ['super_admin', 'tenant_admin'] },
      }],
    })
    await router.push('/gc-control-7x/analytics/popularity?tenant_id=999')
    await router.isReady()

    const wrapper = shallowMount(AnalyticsPopularity, {
      global: {
        plugins: [pinia, router],
        stubs: {
          AnalyticsNavigation: true,
          AnalyticsEmptyState: true,
          ProductImage: true,
          TenantAnalyticsFilter: true,
          'el-table': { template: '<div />' },
          'el-table-column': { template: '<div />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="analytics-popularity"]').text()).toContain('Analytics Product Ranking')
    expect(apiMocks.getProductPopularity).toHaveBeenCalledWith(
      expect.objectContaining({ tenant_id: undefined, limit: 20 }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(router.currentRoute.value.matched[0].meta.roles).toEqual(['super_admin', 'tenant_admin'])
  })
})
