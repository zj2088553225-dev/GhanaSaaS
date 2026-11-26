<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, watch } from 'vue'
import {
  getPublicStorefrontTracking,
  type StorefrontTrackingSetting,
} from '../../api/storefrontTracking'
import { isAdminFrontendPath, isLegacyAdminEntryPath } from '../../utils/adminPath.ts'

interface MetaPixelFunction {
  (...args: unknown[]): void
  callMethod?: (...args: unknown[]) => void
  queue: unknown[][]
  push: MetaPixelFunction
  loaded: boolean
  version: string
}

declare global {
  interface Window {
    fbq?: MetaPixelFunction
    _fbq?: MetaPixelFunction
  }
}

const props = defineProps<{
  path: string
  fullPath: string
}>()
let setting: StorefrontTrackingSetting | null = null
let injectedCustomHeadNodes: Node[] = []

onMounted(async () => {
  try {
    setting = await getPublicStorefrontTracking()
    applyTrackingSetting(setting)
    await nextTick()
    trackPageView()
  } catch {
    // Tracking must never block storefront rendering.
  }
})

watch(
  () => props.fullPath,
  async () => {
    if (!isStorefrontPath(props.path)) {
      return
    }
    await nextTick()
    trackPageView()
  },
)

onUnmounted(() => {
  removeMetaPixel()
  clearCustomHead()
})

function applyTrackingSetting(nextSetting: StorefrontTrackingSetting) {
  removeMetaPixel()
  clearCustomHead()

  if (nextSetting.meta_pixel_enabled && /^[0-9]{5,32}$/.test(nextSetting.meta_pixel_id)) {
    installMetaPixel(nextSetting.meta_pixel_id)
  }
  if (nextSetting.custom_head_enabled && nextSetting.custom_head_html.trim()) {
    injectCustomHead(nextSetting.custom_head_html)
  }
}

function installMetaPixel(pixelID: string) {
  if (!window.fbq) {
    const fbq = function (...args: unknown[]) {
      if (fbq.callMethod) {
        fbq.callMethod(...args)
      } else {
        fbq.queue.push(args)
      }
    } as MetaPixelFunction
    fbq.queue = []
    fbq.push = fbq
    fbq.loaded = true
    fbq.version = '2.0'
    window.fbq = fbq
    window._fbq = fbq
  }

  let script = document.getElementById('meta-pixel-script') as HTMLScriptElement | null
  if (!script) {
    script = document.createElement('script')
    script.id = 'meta-pixel-script'
    script.async = true
    script.src = 'https://connect.facebook.net/en_US/fbevents.js'
    document.head.appendChild(script)
  }
  script.dataset.pixelId = pixelID
  window.fbq?.('init', pixelID)
}

function trackPageView() {
  if (!setting?.meta_pixel_enabled || !isStorefrontPath(props.path)) {
    return
  }
  window.fbq?.('track', 'PageView')
}

function removeMetaPixel() {
  document.getElementById('meta-pixel-script')?.remove()
  delete window.fbq
  delete window._fbq
}

function injectCustomHead(html: string) {
  const template = document.createElement('template')
  template.innerHTML = html

  for (const node of Array.from(template.content.childNodes)) {
    const injected = recreateHeadNode(node)
    if (!injected) {
      continue
    }
    if (injected instanceof Element) {
      injected.setAttribute('data-storefront-custom-head', 'true')
    }
    document.head.appendChild(injected)
    injectedCustomHeadNodes.push(injected)
  }
}

function recreateHeadNode(node: Node): Node | null {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent?.trim() ? document.createTextNode(node.textContent) : null
  }
  if (!(node instanceof HTMLScriptElement)) {
    return node.cloneNode(true)
  }

  const script = document.createElement('script')
  for (const attribute of Array.from(node.attributes)) {
    script.setAttribute(attribute.name, attribute.value)
  }
  script.textContent = node.textContent
  return script
}

function clearCustomHead() {
  for (const node of injectedCustomHeadNodes) {
    node.parentNode?.removeChild(node)
  }
  injectedCustomHeadNodes = []
}

function isStorefrontPath(path: string) {
  return !isAdminFrontendPath(path) && !isLegacyAdminEntryPath(path)
}
</script>

<template>
  <span class="hidden" aria-hidden="true" />
</template>
