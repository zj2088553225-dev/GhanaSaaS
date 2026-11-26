import { createRouter, createWebHistory } from 'vue-router'
import { getStoredToken, getStoredUser } from '../stores/auth'
import { getStoredCustomerToken } from '../stores/customerAuth'
import {
  isCustomerAuthRouteUnavailable,
  isPasswordResetRouteUnavailable,
  storefrontAuthEnabled,
} from '../config/storefrontAuth'
import { reservationEnabled } from '../config/storefront'
import { applySeoMeta, routeSeoMeta } from '../composables/useSeoMeta'
import { ADMIN_BASE_PATH, ADMIN_LOGIN_PATH, adminPath, isAdminFrontendPath } from '../utils/adminPath.ts'
import i18n, { STORAGE_KEY } from '../i18n'
import { DEFAULT_LOCALE, currentLocale, isSupportedLocale, stripLocalePrefix, withLocalePath } from '../utils/localePath.ts'
import { parseProductIdFromSeoParam } from '../utils/productSeoUrl.ts'
import { beginStorefrontPageLifecycle, trackStorefrontPageView, type StorefrontEventInput } from '../utils/storefrontAnalytics.ts'

const PlatformHome = () => import('../views/PlatformHome.vue')
const MallProductList = () => import('../views/MallProductList.vue')
const LoginView = () => import('../views/Login.vue')
const NotFound = () => import('../views/NotFound.vue')

const adminRoleAccess: Record<string, string[]> = {
  super_admin: [
    adminPath('/global-dashboard'),
    adminPath('/analytics'),
    adminPath('/tenants'),
    adminPath('/merchant-leads'),
    adminPath('/platform-categories'),
    adminPath('/self-operated-products'),
    adminPath('/site-pages/home'),
    adminPath('/storefront-tracking'),
    ...(reservationEnabled ? [adminPath('/reservation-products'), adminPath('/reservations')] : []),
  ],
  tenant_admin: [adminPath('/dashboard'), adminPath('/analytics'), adminPath('/products'), adminPath('/shop-categories'), adminPath('/orders'), adminPath('/employees'), adminPath('/shop-profile'), adminPath('/settings/developer')],
  admin: [adminPath('/dashboard'), adminPath('/analytics'), adminPath('/products'), adminPath('/shop-categories'), adminPath('/orders'), adminPath('/employees'), adminPath('/shop-profile'), adminPath('/settings/developer')],
  finance: [adminPath('/dashboard'), adminPath('/orders')],
  picker: [adminPath('/orders')],
}

const knownAdminRoles = new Set(Object.keys(adminRoleAccess))
function isKnownAdminRole(role?: string) {
  return Boolean(role && knownAdminRoles.has(role))
}

function adminLandingPath(role?: string) {
  if (role === 'super_admin') {
    return adminPath('/global-dashboard')
  }
  if (role === 'picker') {
    return adminPath('/orders')
  }
  return adminPath('/dashboard')
}

function pathStartsWithRoute(path: string, routePath: string) {
  return path === routePath || path.startsWith(`${routePath}/`)
}

function isReservationStorefrontPath(path: string) {
  return pathStartsWithRoute(stripLocalePrefix(path), '/reservations')
}

function canAccessAdminPath(path: string, role?: string) {
  if (!isAdminFrontendPath(path) || path === ADMIN_BASE_PATH) {
    return true
  }

  if (!isKnownAdminRole(role)) {
    return false
  }

  const allowedRoutes = role ? adminRoleAccess[role] : undefined
  if (!allowedRoutes) {
    return false
  }
  return allowedRoutes.some((routePath) => pathStartsWithRoute(path, routePath))
}

function legacyStorefrontRedirect(to: any) {
  return {
    path: withLocalePath(DEFAULT_LOCALE, to.path),
    query: to.query,
    hash: to.hash,
  }
}

function localizedStorefrontPath(to: any, path: string) {
  return withLocalePath(currentLocale(to), path)
}

function stringRouteValue(value: unknown) {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

function storefrontPageViewContext(to: any): Omit<StorefrontEventInput, 'event_name'> | null {
  const locale = currentLocale(to)
  const pagePath = to.path
  switch (String(to.name || '')) {
    case 'home':
    case 'platform-home':
      return { source_type: 'direct', source_id: 'home', page_type: 'home', page_path: pagePath, locale }
    case 'mall-product-list': {
      const keyword = stringRouteValue(to.query.keyword).trim()
      const categoryID = stringRouteValue(to.query.platform_category_id).trim()
      if (keyword) {
        return { source_type: 'search', source_id: 'results', page_type: 'search', page_path: pagePath, locale }
      }
      if (/^[1-9]\d*$/.test(categoryID)) {
        return { source_type: 'category', source_id: categoryID, page_type: 'mall', page_path: pagePath, locale }
      }
      const source = stringRouteValue(to.query.source).trim().toLowerCase()
      return { source_type: 'mall', source_id: source === 't6' || source === 'merchant' ? source : 'all', page_type: 'mall', page_path: pagePath, locale }
    }
    case 'product-detail': {
      const productID = parseProductIdFromSeoParam(stringRouteValue(to.params.productParam))
      if (!productID) {
        return null
      }
      return {
        product_id: productID,
        source_type: 'product_detail',
        source_id: String(productID),
        page_type: 'product',
        page_path: pagePath,
        locale,
      }
    }
    case 'shop-home': {
      const slug = stringRouteValue(to.params.slug).trim()
      return slug ? { entity_type: 'shop', entity_id: slug, source_type: 'shop', source_id: slug, page_type: 'shop', page_path: pagePath, locale } : null
    }
    case 'shop-list':
      return { source_type: 'mall', source_id: 'shops', page_type: 'mall', page_path: pagePath, locale }
    case 'merchant-join':
      return { source_type: 'direct', source_id: 'merchant_join', page_type: 'other', page_path: pagePath, locale }
    case 'cart':
    case 'store-login':
    case 'checkout':
    case 'my-orders':
    case 'order-success':
      return { source_type: 'direct', source_id: String(to.name), page_type: 'other', page_path: pagePath, locale }
    default:
      return null
  }
}

function syncRouteLocale(to: any) {
  const localeParam = to.params.locale
  const locale = Array.isArray(localeParam) ? localeParam[0] : localeParam
  if (!isSupportedLocale(locale)) {
    return
  }
  if (i18n.global.locale.value !== locale) {
    i18n.global.locale.value = locale
  }
  localStorage.setItem(STORAGE_KEY, locale)
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/mall',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/products/:productParam',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/reservations',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/reservations/mine',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/reservations/:slug',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/shops',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/shops/:slug',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/merchant/join',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/platform-home',
      redirect: { path: withLocalePath(DEFAULT_LOCALE, '/') },
    },
    {
      path: '/cart',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/store-login',
      redirect: (to) => ({
        path: withLocalePath(DEFAULT_LOCALE, '/login'),
        query: to.query,
        hash: to.hash,
      }),
    },
    {
      path: '/login',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/register',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/forgot-password',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/reset-password',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/account',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/checkout',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/orders',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/orders/success',
      redirect: legacyStorefrontRedirect,
    },
    {
      path: '/:locale(en|zh)',
      name: 'home',
      component: PlatformHome,
    },
    {
      path: '/:locale(en|zh)/mall',
      name: 'mall-product-list',
      component: MallProductList,
    },
    {
      path: '/:locale(en|zh)/products/:productParam',
      name: 'product-detail',
      component: () => import('../views/ProductDetail.vue'),
    },
    {
      path: '/:locale(en|zh)/reservations',
      name: 'reservation-product-list',
      component: () => import('../views/reservations/ReservationProductList.vue'),
    },
    {
      path: '/:locale(en|zh)/reservations/mine',
      name: 'my-anonymous-reservations',
      component: () => import('../views/reservations/MyAnonymousReservations.vue'),
    },
    {
      path: '/:locale(en|zh)/reservations/:slug',
      name: 'reservation-product-detail',
      component: () => import('../views/reservations/ReservationProductDetail.vue'),
    },
    {
      path: '/:locale(en|zh)/shops',
      name: 'shop-list',
      component: () => import('../views/ShopList.vue'),
    },
    {
      path: '/:locale(en|zh)/merchant/join',
      name: 'merchant-join',
      component: () => import('../views/storefront/MerchantJoin.vue'),
    },
    {
      path: '/:locale(en|zh)/shops/:slug',
      name: 'shop-home',
      component: () => import('../views/ShopHome.vue'),
    },
    {
      path: '/:locale(en|zh)/platform-home',
      name: 'platform-home',
      component: PlatformHome,
    },
    {
      path: '/:locale(en|zh)/cart',
      name: 'cart',
      component: () => import('../views/Cart.vue'),
    },
    {
      path: '/:locale(en|zh)/store-login',
      redirect: (to) => ({
        path: localizedStorefrontPath(to, '/login'),
        query: to.query,
        hash: to.hash,
      }),
    },
    {
      path: '/:locale(en|zh)/login',
      name: 'customer-login',
      component: () => import('../views/auth/CustomerLogin.vue'),
    },
    {
      path: '/:locale(en|zh)/register',
      name: 'customer-register',
      component: () => import('../views/auth/Register.vue'),
    },
    {
      path: '/:locale(en|zh)/forgot-password',
      name: 'forgot-password',
      component: () => import('../views/auth/ForgotPassword.vue'),
    },
    {
      path: '/:locale(en|zh)/reset-password',
      name: 'reset-password',
      component: () => import('../views/auth/ResetPassword.vue'),
    },
    {
      path: '/:locale(en|zh)/account',
      name: 'customer-account',
      component: () => import('../views/Account.vue'),
      meta: { requiresCustomerAuth: true },
    },
    {
      path: '/:locale(en|zh)/checkout',
      name: 'checkout',
      component: () => import('../views/Checkout.vue'),
      meta: { requiresCustomerAuth: true },
    },
    {
      path: '/:locale(en|zh)/orders',
      name: 'my-orders',
      component: () => import('../views/storefront/MyOrders.vue'),
      meta: { requiresCustomerAuth: true },
    },
    {
      path: '/:locale(en|zh)/orders/success',
      name: 'order-success',
      component: () => import('../views/OrderSuccess.vue'),
      meta: { requiresCustomerAuth: true },
    },
    {
      path: ADMIN_LOGIN_PATH,
      name: 'admin-login',
      component: LoginView,
    },
    {
      path: '/403',
      name: 'forbidden',
      component: () => import('../views/Forbidden.vue'),
    },
    {
      path: '/admin',
      name: 'legacy-admin-root',
      component: NotFound,
    },
    {
      path: '/admin/:pathMatch(.*)*',
      name: 'legacy-admin-entry',
      component: NotFound,
    },
    {
      path: ADMIN_BASE_PATH,
      component: () => import('../layouts/AdminLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          redirect: adminPath('/dashboard'),
        },
        {
          path: 'global-dashboard',
          name: 'admin-global-dashboard',
          component: () => import('../views/admin/GlobalDashboard.vue'),
        },
        {
          path: 'analytics',
          name: 'admin-analytics-overview',
          component: () => import('../views/admin/analytics/AnalyticsOverview.vue'),
        },
        {
          path: 'analytics/products',
          name: 'admin-analytics-products',
          component: () => import('../views/admin/analytics/AnalyticsProducts.vue'),
        },
        {
          path: 'analytics/popularity',
          name: 'admin-analytics-popularity',
          component: () => import('../views/admin/analytics/AnalyticsPopularity.vue'),
          meta: { roles: ['super_admin', 'tenant_admin'] },
        },
        {
          path: 'analytics/products/:id',
          name: 'admin-analytics-product-detail',
          component: () => import('../views/admin/analytics/AnalyticsProductDetail.vue'),
        },
        {
          path: 'analytics/content',
          name: 'admin-analytics-content',
          component: () => import('../views/admin/analytics/AnalyticsContent.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'analytics/content/:trackingKey',
          name: 'admin-analytics-content-detail',
          component: () => import('../views/admin/analytics/AnalyticsContentDetail.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'tenants',
          name: 'admin-tenants',
          component: () => import('../views/admin/tenants/TenantList.vue'),
        },
        {
          path: 'merchant-leads',
          name: 'admin-merchant-leads',
          component: () => import('../views/admin/MerchantLeads.vue'),
        },
        {
          path: 'platform-categories',
          name: 'admin-platform-categories',
          component: () => import('../views/admin/PlatformCategories.vue'),
        },
        {
          path: 'self-operated-products',
          name: 'admin-self-operated-products',
          redirect: adminPath('/tenants'),
        },
        {
          path: 'site-pages/home',
          name: 'admin-site-page-home',
          component: () => import('../views/admin/SitePageHomeEditor.vue'),
        },
        {
          path: 'storefront-tracking',
          name: 'admin-storefront-tracking',
          component: () => import('../views/admin/settings/StorefrontTracking.vue'),
        },
        {
          path: 'reservation-products',
          name: 'admin-reservation-products',
          component: () => import('../views/admin/reservations/ReservationProductList.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'reservations',
          name: 'admin-reservations',
          component: () => import('../views/admin/reservations/ReservationRecordList.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'reservation-products/create',
          name: 'admin-reservation-product-create',
          component: () => import('../views/admin/reservations/ReservationProductForm.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'reservation-products/:id/edit',
          name: 'admin-reservation-product-edit',
          component: () => import('../views/admin/reservations/ReservationProductForm.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'reservation-products/:id/analytics',
          name: 'admin-reservation-product-analytics',
          component: () => import('../views/admin/reservations/ReservationProductAnalytics.vue'),
          meta: { roles: ['super_admin'] },
        },
        {
          path: 'dashboard',
          name: 'admin-dashboard',
          component: () => import('../views/admin/Dashboard.vue'),
        },
        {
          path: 'products',
          name: 'admin-products',
          component: () => import('../views/admin/products/ProductList.vue'),
        },
        {
          path: 'products/create',
          name: 'admin-product-create',
          component: () => import('../views/admin/products/ProductForm.vue'),
        },
        {
          path: 'products/:id/edit',
          name: 'admin-product-edit',
          component: () => import('../views/admin/products/ProductForm.vue'),
        },
        {
          path: 'shop-categories',
          name: 'admin-shop-categories',
          component: () => import('../views/admin/ShopCategories.vue'),
        },
        {
          path: 'orders',
          name: 'admin-orders',
          component: () => import('../views/admin/orders/OrderList.vue'),
        },
        {
          path: 'employees',
          name: 'admin-employees',
          component: () => import('../views/admin/employees/EmployeeList.vue'),
        },
        {
          path: 'shop-profile',
          name: 'admin-shop-profile',
          component: () => import('../views/admin/ShopProfile.vue'),
        },
        {
          path: 'settings/developer',
          name: 'admin-developer-settings',
          component: () => import('../views/admin/settings/Developer.vue'),
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: NotFound,
    },
  ],
})

router.beforeEach((to) => {
  syncRouteLocale(to)

  if (!reservationEnabled && isReservationStorefrontPath(to.path)) {
    return { path: localizedStorefrontPath(to, '/') }
  }

  if (isCustomerAuthRouteUnavailable(to.name)) {
    return { path: localizedStorefrontPath(to, '/') }
  }

  if (isPasswordResetRouteUnavailable(to.name)) {
    return { path: localizedStorefrontPath(to, '/') }
  }

  const isAdminAuthenticated = Boolean(getStoredToken())
  const adminUser = getStoredUser()
  const adminRole = adminUser?.role
  const isCustomerAuthenticated = Boolean(getStoredCustomerToken())
  const requiresAuth = to.matched.some((record) => record.meta.requiresAuth)
  const requiresCustomerAuth = to.matched.some((record) => record.meta.requiresCustomerAuth)

  if (requiresAuth && !isAdminAuthenticated) {
    return { path: ADMIN_LOGIN_PATH, query: { redirect: to.fullPath } }
  }

  if (requiresAuth && !isKnownAdminRole(adminRole)) {
    return { path: '/403' }
  }

  if (requiresAuth && to.path === ADMIN_BASE_PATH) {
    return { path: adminLandingPath(adminRole) }
  }

  if (requiresAuth && !canAccessAdminPath(to.path, adminRole)) {
    return { path: adminLandingPath(adminRole) }
  }

  const routeRoles = to.meta.roles
  if (requiresAuth && Array.isArray(routeRoles) && (!adminRole || !(routeRoles as string[]).includes(adminRole))) {
    return { path: adminLandingPath(adminRole) }
  }

  if (storefrontAuthEnabled && requiresCustomerAuth) {
    if (!isCustomerAuthenticated) {
      return { path: localizedStorefrontPath(to, '/login'), query: { redirect: to.fullPath } }
    }
  }

  if (to.path === ADMIN_LOGIN_PATH && isAdminAuthenticated) {
    if (!isKnownAdminRole(adminRole)) {
      return { path: '/403' }
    }
    return { path: adminLandingPath(adminRole) }
  }

  if (isCustomerAuthenticated &&
    (to.name === 'customer-login' || to.name === 'customer-register')) {
    return { path: localizedStorefrontPath(to, '/account') }
  }
})

router.afterEach((to, _from, failure) => {
  applySeoMeta(routeSeoMeta(to.fullPath))
  if (failure) {
    return
  }
  const pageView = storefrontPageViewContext(to)
  if (pageView) {
    const navigationKey = pageView.product_id
      ? `product:${pageView.locale}:${pageView.product_id}`
      : to.fullPath
    trackStorefrontPageView(navigationKey, pageView)
  } else {
    beginStorefrontPageLifecycle(`excluded:${to.fullPath}`)
  }
})

export default router
