<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getStorefrontPlatformCategoryChildren, type PlatformCategory } from '../../api/platformCategories'
import { rootPlatformCategories } from '../../utils/homeStorefront'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const props = defineProps<{
  categories: PlatformCategory[]
  compact?: boolean
}>()
const { t } = useI18n()
const route = useRoute()
const activeLocale = computed(() => currentLocale(route))
const childrenByParent = ref<Record<number, PlatformCategory[]>>({})
const loadingParent = ref<Record<number, boolean>>({})

watch(activeLocale, () => {
  childrenByParent.value = {}
  loadingParent.value = {}
})

function categoryLink(category: PlatformCategory) {
  return withLocalePath(activeLocale.value, `/mall?platform_category_id=${category.id}`)
}

function topLevelCategories() {
  return rootPlatformCategories(props.categories)
}

function categoryHasChildren(category: PlatformCategory) {
  return category.has_children || !category.is_leaf
}

async function loadChildren(category: PlatformCategory) {
  if (!categoryHasChildren(category) || childrenByParent.value[category.id] || loadingParent.value[category.id]) {
    return
  }
  loadingParent.value[category.id] = true
  try {
    childrenByParent.value[category.id] = await getStorefrontPlatformCategoryChildren(category.id, { maxLevel: 2 })
  } catch {
    childrenByParent.value[category.id] = []
  } finally {
    loadingParent.value[category.id] = false
  }
}
</script>

<template>
  <!-- Desktop / tablet vertical nav -->
  <nav v-if="!compact" class="hidden h-full rounded-xl bg-white shadow-sm xl:block">
    <div class="border-b border-slate-100 px-4 py-3">
      <h3 class="text-sm font-black text-slate-950">{{ t('home.categories') }}</h3>
    </div>
    <ul class="py-2">
      <li
        v-for="category in topLevelCategories()"
        :key="category.id"
        class="group relative"
        @mouseenter="loadChildren(category)"
        @focusin="loadChildren(category)"
      >
        <RouterLink
          :to="categoryLink(category)"
          class="flex items-center justify-between px-4 py-2.5 text-sm font-semibold text-slate-700 transition hover:bg-orange-50 hover:text-orange-700"
        >
          <span class="truncate">{{ category.name }}</span>
          <svg v-if="categoryHasChildren(category)" xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
        </RouterLink>

        <!-- Hover popup -->
        <div
          v-if="categoryHasChildren(category)"
          class="invisible absolute left-full top-0 z-20 ml-1 w-64 rounded-xl border border-slate-100 bg-white p-3 opacity-0 shadow-xl transition-opacity group-hover:visible group-hover:opacity-100"
        >
          <p class="mb-2 text-xs font-black uppercase tracking-wider text-slate-400">{{ category.name }}</p>
          <div class="flex flex-wrap gap-2">
            <RouterLink
              v-for="child in childrenByParent[category.id] || []"
              :key="child.id"
              :to="categoryLink(child)"
              class="rounded-lg px-2.5 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-orange-50 hover:text-orange-700"
            >
              {{ child.name }}
            </RouterLink>
            <span v-if="loadingParent[category.id]" class="px-2.5 py-1.5 text-xs font-semibold text-slate-400">
              {{ t('home.loading') }}
            </span>
            <span v-else-if="childrenByParent[category.id] && !childrenByParent[category.id].length" class="px-2.5 py-1.5 text-xs text-slate-400">
              {{ t('home.empty') }}
            </span>
          </div>
        </div>
      </li>
    </ul>
  </nav>

  <!-- Mobile horizontal nav -->
  <nav v-if="compact" class="xl:hidden">
    <div class="flex gap-2 overflow-x-auto pb-2 pt-1 scrollbar-hide">
      <RouterLink
        v-for="category in topLevelCategories()"
        :key="category.id"
        :to="categoryLink(category)"
        class="shrink-0 rounded-full bg-white px-3.5 py-1.5 text-xs font-bold text-slate-700 shadow-sm"
      >
        {{ category.name }}
      </RouterLink>
    </div>
  </nav>
</template>

<style scoped>
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
</style>
