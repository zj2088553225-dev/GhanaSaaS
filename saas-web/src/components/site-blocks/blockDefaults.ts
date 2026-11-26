import type { SitePageBlock, SitePageBlockItem, SitePageBlockSlide, SitePageBlockType } from '../../api/sitePages'

const DEFAULT_LIMIT = 8

export function createTrackingKey() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  const bytes = new Uint8Array(16)
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    crypto.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256)
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function trackingKey(value?: string) {
  return value?.trim() || createTrackingKey()
}

function trimText(value?: string) {
  return value?.trim() || ''
}

function cloneItems(items?: SitePageBlockItem[]): SitePageBlockItem[] {
  return (items || []).map((item) => ({
    ...item,
    tracking_key: trackingKey(item.tracking_key),
    links: cloneItems(item.links),
  }))
}

function cloneSlides(slides?: SitePageBlockSlide[]): SitePageBlockSlide[] {
  return (slides || []).map((slide) => ({
    ...slide,
    tracking_key: trackingKey(slide.tracking_key),
  }))
}

function normalizeProductFloorSource(source?: SitePageBlock['source']): 'all' | 't6' | 'merchant' {
  switch (source) {
    case 't6':
    case 'featured':
      return 't6'
    case 'merchant':
      return 'merchant'
    default:
      return 'all'
  }
}

function normalizeProductFloorSourceType(
  sourceType: SitePageBlock['source_type'],
  platformCategoryID?: number,
): NonNullable<SitePageBlock['source_type']> {
  switch (sourceType) {
    case 'popular':
    case 'latest':
    case 'category':
    case 'manual':
      return sourceType
    default:
      return platformCategoryID ? 'category' : 'latest'
  }
}

function normalizeProductGridSource(source?: SitePageBlock['source']): 'latest' | 'featured' | 'hot' | 'category' {
  switch (source) {
    case 'featured':
    case 'hot':
    case 'category':
      return source
    default:
      return 'latest'
  }
}

export function createDefaultBlock(type: SitePageBlockType): SitePageBlock {
  const tracking_key = createTrackingKey()
  switch (type) {
    case 'hero':
      return {
        type,
        tracking_key,
        title: 'Ghana Chinatown',
        subtitle: '一站式本地商城服务平台',
        backgroundImage: '',
        buttonText: '浏览商家',
        buttonLink: '/shops',
      }
    case 'hero_banner':
      return {
        type,
        tracking_key,
        title: 'Ghana Chinatown Marketplace',
        subtitle: 'Shop phones, computers, fashion and daily essentials from trusted local sellers.',
        image_url: '',
        link_url: '/mall',
        button_text: 'Shop Now',
      }
    case 'hero_carousel':
      return {
        type,
        tracking_key,
        title: '',
        autoplay: true,
        interval: 4000,
        height: 'large',
        fit: 'cover',
        slides: [
          {
            tracking_key: createTrackingKey(),
            image: '',
            link: '/mall',
            enabled: true,
          },
        ],
      }
    case 'category_nav':
      return {
        type,
        tracking_key,
        title: 'Shop by Category',
        subtitle: '',
        platform_category_id: undefined,
        platform_category_path: '',
        items: [],
      }
    case 'map_section':
      return {
        type,
        tracking_key,
        enabled: true,
        title_en: 'Visit Us',
        title_zh: '到访我们',
        subtitle_en: 'Find our location on Google Maps',
        subtitle_zh: '在 Google 地图上查看我们的位置',
        place_query: 'Accra, Ghana',
        place_id: '',
        map_height: 320,
        external_map_url: '',
        sort_order: 2,
      }
    case 'image_text':
      return {
        type,
        tracking_key,
        title: '连接本地商家与消费者',
        content: '帮助消费者发现本地优质商家，也让商家拥有稳定、清晰的线上展示与交易入口。',
        image: '',
        layout: 'image_left',
        buttonText: '浏览商家',
        buttonLink: '/shops',
      }
    case 'feature_grid':
      return {
        type,
        tracking_key,
        title: '服务优势',
        subtitle: '面向本地生活和零售场景的一站式平台能力',
        items: [
          { tracking_key: createTrackingKey(), title: '商家展示', description: '集中展示商家主页、商品和服务信息。', icon: '店' },
          { tracking_key: createTrackingKey(), title: '本地配送', description: '围绕本地订单、支付和履约流程搭建。', icon: '配' },
          { tracking_key: createTrackingKey(), title: '统一入口', description: '消费者可以从一个平台发现更多本地选择。', icon: '选' },
        ],
      }
    case 'product_grid':
      return {
        type,
        tracking_key,
        title: 'Latest Products',
        subtitle: '',
        source: 'latest',
        platform_category_id: undefined,
        platform_category_path: '',
        limit: DEFAULT_LIMIT,
      }
    case 'product_floor':
      return {
        type,
        tracking_key,
        title: 'Popular Products',
        subtitle: 'Ranked by real customer activity',
        source_type: 'latest',
        source: 'all',
        platform_category_id: undefined,
        platform_category_path: '',
        limit: 10,
        columns: 5,
        sort: 'latest',
        enabled: true,
        show_more_link: true,
      }
    case 'shop_grid':
      return { type, tracking_key, title: '推荐商家', subtitle: '', limit: DEFAULT_LIMIT }
    case 'promo_banner':
      return {
        type,
        tracking_key,
        title: 'Deals and New Arrivals',
        subtitle: 'Discover fresh goods and seasonal promotions',
        image: '',
        image_url: '',
        link_url: '/mall',
        buttonText: 'Shop Deals',
        buttonLink: '/mall',
      }
    case 'footer_links':
      return {
        type,
        tracking_key,
        title: 'Footer Links',
        items: [
          {
            tracking_key: createTrackingKey(),
            title: 'Marketplace',
            links: [
              { tracking_key: createTrackingKey(), name: 'Shop All', link: '/mall' },
              { tracking_key: createTrackingKey(), name: 'Stores', link: '/shops' },
              { tracking_key: createTrackingKey(), name: 'Cart', link: '/cart' },
            ],
          },
          {
            tracking_key: createTrackingKey(),
            title: 'Support',
            links: [
              { tracking_key: createTrackingKey(), name: 'My Orders', link: '/orders' },
              { tracking_key: createTrackingKey(), name: 'Contact', link: '/' },
            ],
          },
        ],
      }
    case 'contact':
      return {
        type,
        tracking_key,
        title: '联系我们',
        content: '欢迎商家入驻，也欢迎消费者反馈本地购物需求。',
        phone: '',
        whatsapp: '',
        address: '',
      }
    case 'text':
      return { type, tracking_key, title: '标题', content: '请输入文本内容' }
    case 'image':
      return { type, tracking_key, title: '图片', image: '', alt: '' }
  }
}

export function createDefaultHomeBlocks(): SitePageBlock[] {
  const hotProducts = createDefaultBlock('product_floor')
  hotProducts.title = 'Popular Products'
  hotProducts.subtitle = 'Ranked by real customer activity'
  hotProducts.source_type = 'popular'
  hotProducts.limit = 10

  const hotSale = createDefaultBlock('product_floor')
  hotSale.title = 'Hot Sale'
  hotSale.subtitle = 'Fast-moving deals from marketplace sellers'
  hotSale.sort = 'hot'
  hotSale.limit = 10

  const newArrivals = createDefaultBlock('product_floor')
  newArrivals.title = 'New Arrivals'
  newArrivals.subtitle = 'Freshly published products'
  newArrivals.limit = 10

  const phoneParts = createDefaultBlock('product_floor')
  phoneParts.title = 'Phone Parts'
  phoneParts.subtitle = 'Screens, batteries, cases and repair essentials'

  const computer = createDefaultBlock('product_floor')
  computer.title = 'Computer & Accessory'
  computer.subtitle = 'Laptops, parts and office accessories'

  const womenCollections = createDefaultBlock('product_floor')
  womenCollections.title = 'Women Latest Collections'
  womenCollections.subtitle = 'Fashion, beauty and lifestyle picks'

  return [
    createDefaultBlock('hero_carousel'),
    createDefaultBlock('category_nav'),
    createDefaultBlock('map_section'),
    hotProducts,
    hotSale,
    newArrivals,
    createDefaultBlock('promo_banner'),
    phoneParts,
    computer,
    womenCollections,
    createDefaultBlock('footer_links'),
  ]
}

export function cloneBlockWithDefaults(block: SitePageBlock): SitePageBlock {
  const defaultBlock = createDefaultBlock(block.type)
  const cloned = {
    ...defaultBlock,
    ...block,
    items: cloneItems(block.items ?? defaultBlock.items),
    slides: cloneSlides(block.slides ?? defaultBlock.slides),
  }
  if (block.type === 'product_floor') {
    cloned.source_type = normalizeProductFloorSourceType(
      block.source_type,
      block.platform_category_id,
    )
  }
  return cloned
}

export function normalizeLimit(value?: number) {
  const limit = Number(value || DEFAULT_LIMIT)
  if (!Number.isFinite(limit) || limit <= 0) {
    return DEFAULT_LIMIT
  }
  return Math.min(limit, 24)
}

export function prepareBlockForSave(block: SitePageBlock): SitePageBlock {
  const normalized = cloneBlockWithDefaults(block)
  const base: SitePageBlock = {
    type: normalized.type,
    tracking_key: trackingKey(normalized.tracking_key),
    title: trimText(normalized.title),
  }

  switch (normalized.type) {
    case 'hero':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        backgroundImage: trimText(normalized.backgroundImage),
        buttonText: trimText(normalized.buttonText),
        buttonLink: trimText(normalized.buttonLink),
      }
    case 'category_nav':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        platform_category_id: normalized.platform_category_id || undefined,
        platform_category_path: trimText(normalized.platform_category_path),
        items: cloneItems(normalized.items).map((item) => ({
          tracking_key: trackingKey(item.tracking_key),
          name: trimText(item.name),
          image: trimText(item.image),
          link: trimText(item.link),
        })),
      }
    case 'map_section':
      return {
        type: normalized.type,
        tracking_key: trackingKey(normalized.tracking_key),
        enabled: normalized.enabled !== false,
        title_en: trimText(normalized.title_en),
        title_zh: trimText(normalized.title_zh),
        subtitle_en: trimText(normalized.subtitle_en),
        subtitle_zh: trimText(normalized.subtitle_zh),
        place_query: trimText(normalized.place_query),
        place_id: trimText(normalized.place_id),
        map_height: Math.min(Math.max(Math.round(Number(normalized.map_height) || 320), 200), 800),
        external_map_url: trimText(normalized.external_map_url),
        sort_order: Math.min(Math.max(Math.round(Number(normalized.sort_order) || 0), 0), 49),
      }
    case 'hero_banner':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        image_url: trimText(normalized.image_url),
        link_url: trimText(normalized.link_url),
        button_text: trimText(normalized.button_text),
      }
    case 'hero_carousel':
      return {
        ...base,
        autoplay: normalized.autoplay !== false,
        interval: Number(normalized.interval) > 0 ? Number(normalized.interval) : 4000,
        height: ['large', 'medium', 'small'].includes(normalized.height || '') ? normalized.height : 'large',
        fit: normalized.fit === 'contain' ? 'contain' : 'cover',
        slides: (normalized.slides || []).map((slide) => ({
          tracking_key: trackingKey(slide.tracking_key),
          image: trimText(slide.image),
          title: trimText(slide.title),
          subtitle: trimText(slide.subtitle),
          buttonText: trimText(slide.buttonText),
          link: trimText(slide.link),
          enabled: slide.enabled !== false,
        })),
      }
    case 'image_text':
      return {
        ...base,
        content: trimText(normalized.content),
        image: trimText(normalized.image),
        layout: normalized.layout === 'image_right' ? 'image_right' : 'image_left',
        buttonText: trimText(normalized.buttonText),
        buttonLink: trimText(normalized.buttonLink),
      }
    case 'feature_grid':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        items: cloneItems(normalized.items).map((item) => ({
          tracking_key: trackingKey(item.tracking_key),
          title: trimText(item.title),
          description: trimText(item.description),
          icon: trimText(item.icon),
        })),
      }
    case 'product_grid':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        source: normalizeProductGridSource(normalized.source),
        platform_category_id: normalized.platform_category_id || undefined,
        platform_category_path: trimText(normalized.platform_category_path),
        limit: normalizeLimit(normalized.limit),
      }
    case 'product_floor':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        source_type: normalizeProductFloorSourceType(normalized.source_type, normalized.platform_category_id),
        source: normalizeProductFloorSource(normalized.source),
        platform_category_id: normalized.platform_category_id || undefined,
        platform_category_path: trimText(normalized.platform_category_path),
        limit: normalizeLimit(normalized.limit || 10),
        columns: Math.min(Math.max(Number(normalized.columns || 5), 2), 6),
        sort: normalized.sort === 'hot' ? 'hot' : 'latest',
        enabled: normalized.enabled !== false,
        show_more_link: normalized.show_more_link !== false,
      }
    case 'shop_grid':
      return { ...base, subtitle: trimText(normalized.subtitle), limit: normalizeLimit(normalized.limit) }
    case 'promo_banner':
      return {
        ...base,
        subtitle: trimText(normalized.subtitle),
        image: trimText(normalized.image),
        image_url: trimText(normalized.image_url),
        link_url: trimText(normalized.link_url),
        buttonText: trimText(normalized.buttonText),
        buttonLink: trimText(normalized.buttonLink),
      }
    case 'footer_links':
      return {
        ...base,
        items: cloneItems(normalized.items).map((group) => ({
          tracking_key: trackingKey(group.tracking_key),
          title: trimText(group.title),
          links: cloneItems(group.links).map((link) => ({
            tracking_key: trackingKey(link.tracking_key),
            name: trimText(link.name),
            link: trimText(link.link),
          })),
        })),
      }
    case 'contact':
      return {
        ...base,
        content: trimText(normalized.content),
        phone: trimText(normalized.phone),
        whatsapp: trimText(normalized.whatsapp),
        address: trimText(normalized.address),
      }
    case 'text':
      return { ...base, content: trimText(normalized.content) }
    case 'image':
      return { ...base, image: trimText(normalized.image), alt: trimText(normalized.alt) }
  }
}
