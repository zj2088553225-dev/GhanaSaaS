<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCartStore } from '../../stores/cart'
import { usePreviewMode } from '../../composables/usePreviewMode'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const cartStore = useCartStore()
const { showPreviewMode } = usePreviewMode()
const showBackToTop = ref(false)

function updateScrollState() {
  showBackToTop.value = window.scrollY > 320
}

function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => {
  updateScrollState()
  window.addEventListener('scroll', updateScrollState, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('scroll', updateScrollState)
})
</script>

<template>
  <aside class="fixed right-5 top-1/2 z-40 hidden -translate-y-1/2 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white/95 shadow-xl backdrop-blur xl:flex">
    <button type="button" class="floating-bar-item" @click="showPreviewMode">
      <span class="text-lg" aria-hidden="true">💬</span>
      <span>{{ t('floatingBar.service') }}</span>
    </button>

    <RouterLink :to="withLocalePath(currentLocale(route), '/cart')" class="floating-bar-item relative">
      <span class="text-lg" aria-hidden="true">🛒</span>
      <span>{{ t('floatingBar.cart') }}</span>
      <span
        v-if="cartStore.totalQuantity"
        class="absolute right-1.5 top-1.5 min-w-5 rounded-full bg-orange-500 px-1 text-center text-[10px] font-black leading-5 text-white"
      >
        {{ cartStore.totalQuantity > 99 ? '99+' : cartStore.totalQuantity }}
      </span>
    </RouterLink>

    <button type="button" class="floating-bar-item" @click="showPreviewMode">
      <span class="text-lg" aria-hidden="true">📦</span>
      <span>{{ t('floatingBar.orders') }}</span>
    </button>

    <button v-if="showBackToTop" type="button" class="floating-bar-item" @click="scrollToTop">
      <span class="text-lg" aria-hidden="true">↑</span>
      <span>{{ t('floatingBar.top') }}</span>
    </button>
  </aside>
</template>

<style scoped>
.floating-bar-item {
  display: flex;
  width: 64px;
  min-height: 64px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  border-bottom: 1px solid #f1f5f9;
  color: #475569;
  font-size: 11px;
  font-weight: 700;
  transition:
    color 150ms ease,
    background-color 150ms ease;
}

.floating-bar-item:last-child {
  border-bottom: 0;
}

.floating-bar-item:hover {
  background: #fff7ed;
  color: #ea580c;
}
</style>
