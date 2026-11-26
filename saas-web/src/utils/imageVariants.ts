export type ImageVariant = 'thumb' | 'card' | 'large'

const optimizedVariantPattern = /^(.*\/uploads\/images\/\d{4}\/\d{2}\/[a-f0-9]{32})_(thumb|card|large)\.webp$/i
const optimizedOriginalPattern = /^(.*\/uploads\/images\/\d{4}\/\d{2}\/[a-f0-9]{32})_original\.(jpe?g|png|webp)$/i
const optimizedBasePattern = /^(.*\/uploads\/images\/\d{4}\/\d{2}\/[a-f0-9]{32})\.webp$/i

export function imageVariantURL(value: string | null | undefined, variant: ImageVariant): string {
  const url = value?.trim() ?? ''
  if (!url) {
    return ''
  }

  const { base, suffix } = splitURLSuffix(url)
  const variantMatch = base.match(optimizedVariantPattern)
  if (variantMatch) {
    return `${variantMatch[1]}_${variant}.webp${suffix}`
  }

  const originalMatch = base.match(optimizedOriginalPattern)
  if (originalMatch) {
    return `${originalMatch[1]}_${variant}.webp${suffix}`
  }

  const baseMatch = base.match(optimizedBasePattern)
  if (baseMatch) {
    return `${baseMatch[1]}_${variant}.webp${suffix}`
  }

  return url
}

function splitURLSuffix(url: string): { base: string; suffix: string } {
  const index = url.search(/[?#]/)
  if (index === -1) {
    return { base: url, suffix: '' }
  }
  return {
    base: url.slice(0, index),
    suffix: url.slice(index),
  }
}
