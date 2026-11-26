import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const productImage = readFileSync(new URL('../src/components/ProductImage.vue', import.meta.url), 'utf8')
const heroCarousel = readFileSync(new URL('../src/components/site-blocks/HeroCarousel.vue', import.meta.url), 'utf8')

assert.match(productImage, /fallbackSrc\?:\s*string\s*\|\s*null/)
assert.match(productImage, /currentSrc\.value\s*=\s*props\.src\?\.trim\(\)\s*\|\|\s*props\.fallbackSrc\?\.trim\(\)/)
assert.match(productImage, /fallbackSrc\s*!==\s*currentSrc\.value/)
assert.match(productImage, /usingFallback\.value\s*=\s*true/)
assert.match(productImage, /failed\.value\s*=\s*true/)
assert.match(productImage, /\[\(\)\s*=>\s*props\.src,\s*\(\)\s*=>\s*props\.fallbackSrc\]/)

assert.match(heroCarousel, /:src="imageVariantURL\(slide\.image,\s*'large'\)"/)
assert.match(heroCarousel, /:fallback-src="slide\.image"/)
assert.match(heroCarousel, /:loading="index\s*===\s*0\s*\?\s*'eager'\s*:\s*'lazy'"/)
assert.match(heroCarousel, /:fetchpriority="slideFetchPriority\(index\)"/)

console.log('hero carousel image fallback tests passed')
