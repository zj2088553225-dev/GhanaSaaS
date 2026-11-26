import type { SitePageBlockType } from '../../api/sitePages'

export const editableBlockTypeOptions: Array<{ label: string; value: SitePageBlockType }> = [
  { label: '轮播大图', value: 'hero_carousel' },
  { label: '商城大 Banner', value: 'hero_banner' },
  { label: '分类导航', value: 'category_nav' },
  { label: 'Google 地图', value: 'map_section' },
  { label: '商品楼层', value: 'product_floor' },
  { label: '商品推荐', value: 'product_grid' },
  { label: '广告横幅', value: 'promo_banner' },
  { label: '页脚链接', value: 'footer_links' },
  { label: '首屏大图(旧)', value: 'hero' },
  { label: '图文介绍', value: 'image_text' },
  { label: '服务优势', value: 'feature_grid' },
  { label: '商家推荐', value: 'shop_grid' },
]

const blockLabels: Record<SitePageBlockType, string> = {
  hero: '首屏大图',
  hero_banner: '商城大 Banner',
  hero_carousel: '轮播大图',
  category_nav: '分类导航',
  map_section: 'Google 地图区块',
  image_text: '图文介绍',
  feature_grid: '服务优势',
  product_floor: '商品楼层',
  product_grid: '商品推荐',
  shop_grid: '商家推荐',
  promo_banner: '广告横幅',
  footer_links: '页脚链接',
  contact: '联系我们',
  text: '文本',
  image: '图片',
}

export function blockLabel(type: SitePageBlockType) {
  return blockLabels[type] || type
}
