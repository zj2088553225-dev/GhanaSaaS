import assert from 'node:assert/strict'
import { imageVariantURL } from '../src/utils/imageVariants.ts'
import { formatMoney } from '../src/utils/money.ts'
import {
  activeImageForSku,
  buildProductGallery,
  buildProductMediaGallery,
  normalizeSkuSpecs,
  resolveSpecDimensions,
  skuImageURL,
  skuStock,
  validateSkuBatchQuantity,
} from '../src/utils/productSku.ts'

const skus = [
  {
    id: 101,
    specs: { Size: 'S', Color: 'Blue' },
    sellable_stock: 5,
  },
  {
    id: 102,
    specs: { Size: 'M', Color: 'Green' },
    sellable_stock: 3,
  },
]

const dimensions = resolveSpecDimensions(
  [
    { name: 'Size', values: ['S', 'M', 'L'] },
    { name: 'Color', values: ['Blue', 'Green', 'Yellow'] },
  ],
  skus,
)

assert.deepEqual(dimensions, [
  { name: 'Size', values: ['S', 'M', 'L'] },
  { name: 'Color', values: ['Blue', 'Green', 'Yellow'] },
])
assert.deepEqual(normalizeSkuSpecs('{"Size":"S","Color":"Blue"}'), { Size: 'S', Color: 'Blue' })
assert.equal(formatMoney(8000), 'GHS 80.00')

assert.equal(skuStock({ id: 1, sellable_stock: 5, stock: 3 }), 5)
assert.equal(skuStock({ id: 1, stock: 3 }), 3)
assert.equal(skuStock({ id: 1, sellable_stock: 0, stock: 0 }), 0)

const batchSku = { id: 301, specs: { Size: 'L' }, sellable_stock: 5, is_active: true }
assert.equal(validateSkuBatchQuantity(batchSku, 3), 'ok')
assert.equal(validateSkuBatchQuantity(batchSku, 6), 'exceeds_stock')
assert.equal(validateSkuBatchQuantity(batchSku, 3, 3), 'exceeds_stock')
assert.equal(validateSkuBatchQuantity({ ...batchSku, is_active: false }, 1), 'out_of_stock')
assert.equal(validateSkuBatchQuantity({ ...batchSku, sellable_stock: 0 }, 1), 'out_of_stock')
assert.equal(validateSkuBatchQuantity(batchSku, 0), 'ok')

const singleSku = { id: 401, specs: {}, sellable_stock: 8, is_active: true }
assert.equal(validateSkuBatchQuantity(singleSku, 1), 'ok')
assert.equal(validateSkuBatchQuantity(singleSku, 8), 'ok')
assert.equal(validateSkuBatchQuantity(singleSku, 9), 'exceeds_stock')

assert.deepEqual(
  buildProductGallery({
    main_image: ' /uploads/main.webp ',
    images: ['', '/uploads/detail.webp', '/uploads/main.webp'],
    slider_images: JSON.stringify(['/uploads/slide.webp', '/uploads/detail.webp', null]),
  }),
  ['/uploads/main.webp', '/uploads/detail.webp', '/uploads/slide.webp'],
)
assert.deepEqual(buildProductGallery({ mainImage: '/uploads/camel.webp', gallery_images: ['/uploads/gallery.webp'] }), [
  '/uploads/camel.webp',
  '/uploads/gallery.webp',
])
assert.deepEqual(buildProductGallery({}), [])
assert.deepEqual(
  buildProductMediaGallery({
    video_url: '/uploads/videos/2026/06/product.mp4',
    main_image: '/uploads/main.webp',
    slider_images: ['/uploads/slide.webp'],
  }),
  [
    { type: 'video', url: '/uploads/videos/2026/06/product.mp4' },
    { type: 'image', url: '/uploads/main.webp' },
    { type: 'image', url: '/uploads/slide.webp' },
  ],
)
assert.deepEqual(buildProductMediaGallery({ main_image: '/uploads/main.webp' }), [
  { type: 'image', url: '/uploads/main.webp' },
])

assert.equal(skuImageURL({ image_url: '/uploads/sku.webp', image: '/uploads/legacy.webp' }), '/uploads/sku.webp')
assert.equal(skuImageURL({ image: '/uploads/legacy.webp' }), '/uploads/legacy.webp')
assert.equal(activeImageForSku('/uploads/main.webp', { image_url: '/uploads/sku.webp' }), '/uploads/sku.webp')
assert.equal(activeImageForSku('/uploads/main.webp', {}), '/uploads/main.webp')

assert.equal(
  imageVariantURL('/uploads/images/2026/06/abcdef0123456789abcdef0123456789_large.webp', 'card'),
  '/uploads/images/2026/06/abcdef0123456789abcdef0123456789_card.webp',
)
assert.equal(
  imageVariantURL('/uploads/images/2026/06/abcdef0123456789abcdef0123456789_original.png', 'thumb'),
  '/uploads/images/2026/06/abcdef0123456789abcdef0123456789_thumb.webp',
)
assert.equal(imageVariantURL('/uploads/legacy-product.jpg', 'card'), '/uploads/legacy-product.jpg')

console.log('product sku tests passed')
