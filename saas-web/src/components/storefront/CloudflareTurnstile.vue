<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { parseStorefrontFeatureFlag } from '../../config/storefront'
import { loadTurnstileAPI, reservationTurnstileOptions, type TurnstileAPI } from '../../utils/turnstile'

const props = withDefaults(defineProps<{
  enabled?: boolean
  siteKey?: string
  resetKey?: number
  action?: 'reservation_otp' | 'reservation_create'
}>(), {
  enabled: parseStorefrontFeatureFlag(import.meta.env.VITE_TURNSTILE_ENABLED),
  siteKey: String(import.meta.env.VITE_TURNSTILE_SITE_KEY || ''),
  resetKey: 0,
  action: 'reservation_otp',
})

const emit = defineEmits<{
  token: [value: string]
  error: [message: string]
}>()

const container = ref<HTMLElement>()
const loading = ref(false)
const configurationError = ref('')
let api: TurnstileAPI | null = null
let widgetID = ''
let renderGeneration = 0
let disposed = false

async function renderWidget() {
  removeWidget()
  const generation = ++renderGeneration
  emit('token', '')
  configurationError.value = ''
  if (!props.enabled) {
    return
  }
  if (!props.siteKey.trim()) {
    configurationError.value = 'Turnstile site key is not configured.'
    emit('error', configurationError.value)
    return
  }
  loading.value = true
  try {
    const loadedAPI = await loadTurnstileAPI()
    await nextTick()
    if (disposed || generation !== renderGeneration || !container.value) return
    api = loadedAPI
    widgetID = loadedAPI.render(container.value, reservationTurnstileOptions(props.siteKey, {
      success: (token) => {
        if (!disposed && generation === renderGeneration) emit('token', token)
      },
      expired: () => {
        if (disposed || generation !== renderGeneration) return
        emit('token', '')
        reset()
      },
      error: () => {
        if (disposed || generation !== renderGeneration) return
        emit('token', '')
        emit('error', 'Human verification could not be completed. Please try again.')
        reset()
      },
    }, props.action))
  } catch {
    if (!disposed && generation === renderGeneration) emit('error', 'Human verification could not be loaded. Please try again.')
  } finally {
    if (!disposed && generation === renderGeneration) loading.value = false
  }
}

function reset() {
  emit('token', '')
  if (api && widgetID) api.reset(widgetID)
}

function removeWidget() {
  renderGeneration += 1
  if (api && widgetID) api.remove(widgetID)
  widgetID = ''
}

watch(() => props.resetKey, () => reset())
watch(() => [props.enabled, props.siteKey, props.action], () => void renderWidget())
onMounted(() => void renderWidget())
onUnmounted(() => {
  disposed = true
  removeWidget()
})
defineExpose({ reset })
</script>

<template>
  <div class="w-full">
    <div ref="container" class="min-h-0 w-full" data-testid="turnstile-container" />
    <p v-if="loading" class="mt-2 text-xs text-slate-500">Loading human verification…</p>
    <p v-if="configurationError" role="alert" class="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs font-semibold text-red-700">
      {{ configurationError }}
    </p>
  </div>
</template>
