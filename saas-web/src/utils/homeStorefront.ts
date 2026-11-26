import type { PlatformCategory } from '../api/platformCategories'

const legacyDefaultKeys: Record<string, string> = {
  'Latest Products': 'homeBlocks.defaultLatestProducts',
  'Latest Hot Products': 'homeBlocks.popularProducts',
  'Popular Products': 'homeBlocks.popularProducts',
  'Fresh picks from the marketplace': 'homeBlocks.defaultFreshPicks',
  'Popular picks updated daily': 'homeBlocks.defaultPopularDaily',
  'Ranked by real customer activity': 'homeBlocks.popularProductsSubtitle',
  'Hot Sale': 'homeBlocks.defaultHotSale',
  'Fast-moving deals from marketplace sellers': 'homeBlocks.defaultHotSaleSubtitle',
  'New Arrivals': 'homeBlocks.defaultNewArrivals',
  'Freshly published products': 'homeBlocks.defaultNewArrivalsSubtitle',
  'Shop by Category': 'homeBlocks.defaultShopByCategory',
  推荐商家: 'homeBlocks.recommendedMerchants',
  'Deals and New Arrivals': 'homeBlocks.defaultDeals',
  'Discover fresh goods and seasonal promotions': 'homeBlocks.defaultDealsSubtitle',
  'Footer Links': 'homeBlocks.defaultFooterLinks',
  Marketplace: 'homeBlocks.marketplace',
  Support: 'homeBlocks.defaultSupport',
  'Shop All': 'homeBlocks.defaultShopAll',
  Stores: 'homeBlocks.defaultStores',
  Cart: 'nav.cart',
  'My Orders': 'nav.orders',
  Contact: 'homeBlocks.contact',
  服务优势: 'homeBlocks.serviceAdvantages',
  面向本地生活和零售场景的一站式平台能力: 'homeBlocks.defaultAdvantagesSubtitle',
  商家展示: 'homeBlocks.defaultMerchantShowcase',
  '集中展示商家主页、商品和服务信息。': 'homeBlocks.defaultMerchantShowcaseDescription',
  本地配送: 'homeBlocks.defaultLocalDelivery',
  '围绕本地订单、支付和履约流程搭建。': 'homeBlocks.defaultLocalDeliveryDescription',
  统一入口: 'homeBlocks.defaultUnifiedEntry',
  '消费者可以从一个平台发现更多本地选择。': 'homeBlocks.defaultUnifiedEntryDescription',
  一站式本地商城服务平台: 'homeBlocks.defaultHeroSubtitle',
  浏览商家: 'homeBlocks.browseShops',
  'Shop phones, computers, fashion and daily essentials from trusted local sellers.': 'homeBlocks.defaultMarketplaceSubtitle',
  'Shop Now': 'homeBlocks.defaultShopNow',
  连接本地商家与消费者: 'homeBlocks.defaultConnectTitle',
  '帮助消费者发现本地优质商家，也让商家拥有稳定、清晰的线上展示与交易入口。': 'homeBlocks.defaultConnectContent',
  'Shop Deals': 'homeBlocks.defaultShopDeals',
  联系我们: 'homeBlocks.contactUs',
  '欢迎商家入驻，也欢迎消费者反馈本地购物需求。': 'homeBlocks.defaultContactContent',
  'Phone Parts': 'homeBlocks.defaultPhoneParts',
  'Screens, batteries, cases and repair essentials': 'homeBlocks.defaultPhonePartsSubtitle',
  'Computer & Accessory': 'homeBlocks.defaultComputer',
  'Laptops, parts and office accessories': 'homeBlocks.defaultComputerSubtitle',
  'Women Latest Collections': 'homeBlocks.defaultWomenCollections',
  'Fashion, beauty and lifestyle picks': 'homeBlocks.defaultWomenCollectionsSubtitle',
}

export function legacyHomeDefaultTranslationKey(value: string | undefined): string | null {
  return legacyDefaultKeys[value?.trim() || ''] || null
}

export function rootPlatformCategories(categories: PlatformCategory[]): PlatformCategory[] {
  return categories
    .filter((category) => category.parent_id == null)
    .sort((left, right) => left.sort_order - right.sort_order || left.id - right.id)
}

export function homeProductLimit(value?: number): number {
  const parsed = Number(value || 8)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 8
  }
  return Math.min(Math.floor(parsed), 10)
}
