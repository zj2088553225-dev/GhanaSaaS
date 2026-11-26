<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getSuperTenants, type Tenant } from '../../api/superTenants'

const props = defineProps<{
  modelValue: number | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
  change: [value: number | null]
  invalid: []
}>()

const tenants = ref<Tenant[]>([])
const loading = ref(false)
const loaded = ref(false)
const search = ref('')
const selectedID = computed(() => props.modelValue || 0)

const visibleTenants = computed(() => {
  const term = search.value.trim().toLocaleLowerCase()
  const matching = term
    ? tenants.value.filter((tenant) => tenant.name.toLocaleLowerCase().includes(term) || String(tenant.id) === term)
    : tenants.value
  return matching.slice(0, 50)
})

async function loadTenants() {
  if (loaded.value || loading.value) {
    return
  }
  loading.value = true
  try {
    tenants.value = await getSuperTenants()
    loaded.value = true
    if (props.modelValue && !tenants.value.some((tenant) => tenant.id === props.modelValue)) {
      emit('update:modelValue', null)
      emit('invalid')
      ElMessage.warning('租户筛选已失效，已切换为全部租户')
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '租户列表加载失败')
  } finally {
    loading.value = false
  }
}

function handleVisible(visible: boolean) {
  if (visible) {
    void loadTenants()
  }
}

function handleSearch(value: string) {
  search.value = value
}

function handleChange(value: number) {
  const tenantID = value > 0 ? value : null
  emit('update:modelValue', tenantID)
  emit('change', tenantID)
}

onMounted(() => {
  if (props.modelValue) {
    void loadTenants()
  }
})
</script>

<template>
  <div class="min-w-56">
    <label class="mb-1.5 block text-xs font-bold text-slate-500">租户范围</label>
    <el-select
      :model-value="selectedID"
      filterable
      remote
      :remote-method="handleSearch"
      :loading="loading"
      class="w-full"
      aria-label="Analytics 租户筛选"
      @visible-change="handleVisible"
      @change="handleChange"
    >
      <el-option label="全部租户" :value="0" />
      <el-option v-for="tenant in visibleTenants" :key="tenant.id" :label="tenant.name" :value="tenant.id">
        <span>{{ tenant.name }}</span>
        <span class="ml-2 text-xs text-slate-400">#{{ tenant.id }}</span>
      </el-option>
    </el-select>
  </div>
</template>
