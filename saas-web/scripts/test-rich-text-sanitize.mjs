import assert from 'node:assert/strict'
import { sanitizeProductDetailHtml } from '../src/utils/sanitizeProductDetail.ts'

const sanitized = sanitizeProductDetailHtml(
  '<script>alert(1)</script><p onclick="evil()">Normal text</p><img src="/uploads/detail.webp" onerror="evil()"><a href="javascript:alert(1)">Bad link</a>',
)

assert.equal(sanitized.includes('<script'), false)
assert.equal(sanitized.includes('onerror'), false)
assert.equal(sanitized.includes('onclick'), false)
assert.equal(sanitized.includes('javascript:'), false)
assert.equal(sanitized.includes('<img src="/uploads/detail.webp">'), true)
assert.equal(sanitized.includes('<p>Normal text</p>'), true)

const listSanitized = sanitizeProductDetailHtml(
  '<ul class="mso"><li onclick="evil()">Rural Farm Commuting<ol style="color:red"><li>Factory Travel</li></ol></li></ul>',
)

assert.equal(listSanitized.includes('<ul>'), true)
assert.equal(listSanitized.includes('<ol>'), true)
assert.equal(listSanitized.includes('<li>Rural Farm Commuting'), true)
assert.equal(listSanitized.includes('<li>Factory Travel</li>'), true)
assert.equal(listSanitized.includes('onclick'), false)
assert.equal(listSanitized.includes('style='), false)
assert.equal(listSanitized.includes('class='), false)

console.log('rich text sanitize tests passed')
