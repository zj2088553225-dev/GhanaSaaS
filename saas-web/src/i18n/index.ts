import { createI18n } from 'vue-i18n'
import en from '../locales/en'
import zh from '../locales/zh'
import { DEFAULT_LOCALE, SUPPORTED_LOCALES, type SupportedLocale } from '../utils/localePath.ts'

const STORAGE_KEY = 'saas_lang'

function getInitialLocale(): SupportedLocale {
  const storedLocale = localStorage.getItem(STORAGE_KEY)
  return SUPPORTED_LOCALES.includes(storedLocale as SupportedLocale) ? (storedLocale as SupportedLocale) : DEFAULT_LOCALE
}

const i18n = createI18n({
  legacy: false,
  locale: getInitialLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: {
    en,
    zh,
  },
})

export { STORAGE_KEY, type SupportedLocale }
export default i18n
