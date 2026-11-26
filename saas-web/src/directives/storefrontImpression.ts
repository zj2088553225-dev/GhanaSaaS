import type { ObjectDirective } from 'vue'
import { isAdminFrontendPath, isLegacyAdminEntryPath } from '../utils/adminPath'
import {
  currentStorefrontPageLifecycleID,
  trackStorefrontImpression,
  type StorefrontEventInput,
} from '../utils/storefrontAnalytics'

type ImpressionBinding = StorefrontEventInput | null | undefined

interface ImpressionState {
  signature: string
  stop: () => void
}

const states = new WeakMap<HTMLElement, ImpressionState>()
export const STOREFRONT_IMPRESSION_THRESHOLD = 0.15

function isAdminPreview() {
  if (typeof window === 'undefined') {
    return true
  }
  return isAdminFrontendPath(window.location.pathname) || isLegacyAdminEntryPath(window.location.pathname)
}

function observeImpression(element: HTMLElement, event: StorefrontEventInput) {
  let active = true
  const record = () => {
    if (!active || isAdminPreview()) {
      return
    }
    active = false
    trackStorefrontImpression(event)
  }

  if (typeof IntersectionObserver === 'undefined') {
    queueMicrotask(record)
    return () => {
      active = false
    }
  }

  const observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) =>
        entry.isIntersecting && entry.intersectionRatio >= STOREFRONT_IMPRESSION_THRESHOLD)) {
        record()
        observer.disconnect()
      }
    },
    { threshold: STOREFRONT_IMPRESSION_THRESHOLD },
  )
  observer.observe(element)
  return () => {
    active = false
    observer.disconnect()
  }
}

function bindImpression(element: HTMLElement, event: ImpressionBinding) {
  const existing = states.get(element)
  if (!event) {
    existing?.stop()
    states.delete(element)
    return
  }

  const signature = `${currentStorefrontPageLifecycleID()}:${JSON.stringify(event)}`
  if (existing?.signature === signature) {
    return
  }
  existing?.stop()
  states.set(element, { signature, stop: observeImpression(element, event) })
}

export const vStorefrontImpression: ObjectDirective<HTMLElement, ImpressionBinding> = {
  mounted(element, binding) {
    bindImpression(element, binding.value)
  },
  updated(element, binding) {
    bindImpression(element, binding.value)
  },
  unmounted(element) {
    states.get(element)?.stop()
    states.delete(element)
  },
}
