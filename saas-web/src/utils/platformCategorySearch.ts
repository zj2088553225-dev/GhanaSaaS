export type PlatformCategorySearchLocale = 'en' | 'zh' | 'all'

interface PlatformCategorySearchParameterOptions {
  limit: number
  locale: PlatformCategorySearchLocale
  leafOnly?: boolean
}

export function buildPlatformCategorySearchParams(
  keyword: string,
  options: PlatformCategorySearchParameterOptions,
) {
  return {
    keyword,
    limit: options.limit,
    locale: options.locale,
    ...(options.leafOnly ? { leaf_only: true } : {}),
  }
}

export function shouldSchedulePlatformCategorySearch(value: string, isComposing: boolean) {
  return !isComposing && value.trim().length > 0
}

export function isCurrentPlatformCategorySearch(
  requestSequence: number,
  currentSequence: number,
  requestKeyword: string,
  currentKeyword: string,
) {
  return requestSequence === currentSequence && requestKeyword === currentKeyword.trim()
}
