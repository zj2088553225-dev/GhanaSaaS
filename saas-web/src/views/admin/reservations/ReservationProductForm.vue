<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules, UploadFile, UploadFiles, UploadProps, UploadRawFile, UploadUserFile } from 'element-plus'
import RichTextDetailEditor from '../../../components/admin/RichTextDetailEditor.vue'
import ProductImage from '../../../components/ProductImage.vue'
import ReservationHeatBadge from '../../../components/storefront/ReservationHeatBadge.vue'
import {
  createSuperReservationProduct,
  getSuperReservationProduct,
  updateSuperReservationProduct,
  type ReservationHeatLevel,
  type ReservationProductPayload,
  type ReservationProductStatus,
} from '../../../api/reservations'
import { useAuthStore } from '../../../stores/auth'
import { adminPath } from '../../../utils/adminPath'
import { sanitizeProductDetailHtml } from '../../../utils/sanitizeProductDetail'

interface ReservationFormState {
  linked_product_id: number | undefined
  slug: string
  title_en: string
  title_zh: string
  summary_en: string
  summary_zh: string
  description_en: string
  description_zh: string
  main_image_url: string
  gallery: string[]
  status: ReservationProductStatus
  reservation_start_at: string
  reservation_end_at: string
  display_order: number
  is_featured: boolean
  heat_level_override: ReservationHeatLevel | ''
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const formRef = ref<FormInstance>()
const loading = ref(false)
const saving = ref(false)
const activeLanguage = ref<'en' | 'zh'>('en')
const mainImageFiles = ref<UploadUserFile[]>([])
const galleryFiles = ref<UploadUserFile[]>([])
const previewOpen = ref(false)
const previewLocale = ref<'en' | 'zh'>('en')
const uploadAction = '/api/v1/admin/uploads/image'
const uploadHeaders = computed(() => authStore.token ? { Authorization: `Bearer ${authStore.token}` } : {})
const productID = computed(() => Number(Array.isArray(route.params.id) ? route.params.id[0] : route.params.id) || 0)
const editing = computed(() => productID.value > 0)
const previewTitle = computed(() => previewLocale.value === 'zh' ? form.title_zh.trim() || form.title_en.trim() : form.title_en.trim())
const previewSummary = computed(() => previewLocale.value === 'zh' ? form.summary_zh.trim() || form.summary_en.trim() : form.summary_en.trim())
const previewDescription = computed(() => sanitizeProductDetailHtml(previewLocale.value === 'zh' ? form.description_zh || form.description_en : form.description_en))
const previewHeat = computed<ReservationHeatLevel>(() => form.heat_level_override || 'new')

const form = reactive<ReservationFormState>({
  linked_product_id: undefined,
  slug: '',
  title_en: '',
  title_zh: '',
  summary_en: '',
  summary_zh: '',
  description_en: '',
  description_zh: '',
  main_image_url: '',
  gallery: [],
  status: 'draft',
  reservation_start_at: '',
  reservation_end_at: '',
  display_order: 0,
  is_featured: false,
  heat_level_override: '',
})

const rules: FormRules = {
  slug: [
    { required: true, message: '请输入 slug', trigger: 'blur' },
    { pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/, message: '仅支持小写字母、数字和单个短横线分隔', trigger: 'blur' },
  ],
  title_en: [{ required: true, message: '英文标题必填', trigger: 'blur' }],
}

onMounted(() => { if (editing.value) void loadProduct() })

async function loadProduct() {
  loading.value = true
  try {
    const item = await getSuperReservationProduct(productID.value)
    Object.assign(form, {
      linked_product_id: item.linked_product_id || undefined,
      slug: item.slug,
      title_en: item.title_en,
      title_zh: item.title_zh || '',
      summary_en: item.summary_en,
      summary_zh: item.summary_zh || '',
      description_en: item.description_en,
      description_zh: item.description_zh || '',
      main_image_url: item.main_image_url,
      gallery: [...item.gallery],
      status: item.status,
      reservation_start_at: toLocalDateTime(item.reservation_start_at),
      reservation_end_at: toLocalDateTime(item.reservation_end_at),
      display_order: item.display_order,
      is_featured: item.is_featured,
      heat_level_override: item.heat_level_override || '',
    })
    mainImageFiles.value = item.main_image_url ? [urlFile(item.main_image_url)] : []
    galleryFiles.value = item.gallery.map(urlFile)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '预约商品加载失败')
  } finally {
    loading.value = false
  }
}

function normalizeSlug() {
  form.slug = form.slug.trim().toLowerCase().replace(/\s+/g, '-')
}

function toLocalDateTime(value: string | null) {
  if (!value) return ''
  const date = new Date(value)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function toISOString(value: string) {
  return value ? new Date(value).toISOString() : null
}

function optional(value: string) {
  const clean = value.trim()
  return clean || null
}

function payload(): ReservationProductPayload {
  return {
    linked_product_id: form.linked_product_id && form.linked_product_id > 0 ? form.linked_product_id : null,
    slug: form.slug.trim(),
    title_en: form.title_en.trim(),
    title_zh: optional(form.title_zh),
    summary_en: form.summary_en.trim(),
    summary_zh: optional(form.summary_zh),
    description_en: form.description_en,
    description_zh: optional(form.description_zh),
    main_image_url: form.main_image_url.trim(),
    gallery: [...form.gallery],
    status: form.status,
    reservation_start_at: toISOString(form.reservation_start_at),
    reservation_end_at: toISOString(form.reservation_end_at),
    display_order: Number(form.display_order) || 0,
    is_featured: form.is_featured,
    heat_level_override: form.heat_level_override || null,
  }
}

async function save() {
  normalizeSlug()
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  if (form.status === 'scheduled' && !form.reservation_start_at) {
    ElMessage.error('定时开放必须设置开始时间')
    return
  }
  if (form.reservation_start_at && form.reservation_end_at && new Date(form.reservation_end_at) <= new Date(form.reservation_start_at)) {
    ElMessage.error('结束时间必须晚于开始时间')
    return
  }
  saving.value = true
  try {
    const result = editing.value
      ? await updateSuperReservationProduct(productID.value, payload())
      : await createSuperReservationProduct(payload())
    ElMessage.success(editing.value ? '预约商品已更新' : '预约商品已创建')
    await router.replace(adminPath(`/reservation-products/${result.id}/edit`))
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function beforeImageUpload(file: UploadRawFile) {
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
    ElMessage.error('仅支持 JPG、PNG、WEBP 图片')
    return false
  }
  if (file.size > 5 * 1024 * 1024) {
    ElMessage.error('图片不能超过 5MB')
    return false
  }
  return true
}

function uploadURL(response: unknown) {
  return (response as { data?: { url?: string } }).data?.url || ''
}

function urlFile(url: string): UploadUserFile {
  return { name: url.split('/').pop() || url, url }
}

const mainImageSuccess: UploadProps['onSuccess'] = (response, file) => {
  const url = uploadURL(response)
  if (!url) return ElMessage.error('图片上传失败')
  form.main_image_url = url
  mainImageFiles.value = [{ name: file.name, url }]
}

const removeMainImage: UploadProps['onRemove'] = () => {
  form.main_image_url = ''
  mainImageFiles.value = []
}

const gallerySuccess: UploadProps['onSuccess'] = (response, file) => {
  const url = uploadURL(response)
  if (!url) return ElMessage.error('图片上传失败')
  if (!form.gallery.includes(url)) form.gallery.push(url)
  file.url = url
}

const galleryRemove: UploadProps['onRemove'] = (_file: UploadFile, files: UploadFiles) => {
  form.gallery = files.map((item) => item.url || '').filter(Boolean)
  galleryFiles.value = files
}

function preview() {
  previewOpen.value = true
}
</script>

<template>
  <section v-loading="loading" class="space-y-5">
    <header class="flex flex-col gap-4 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm sm:flex-row sm:items-center sm:justify-between">
      <div>
        <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">Reservation Product</p>
        <h1 class="mt-1 text-2xl font-black text-slate-950">{{ editing ? '编辑测品' : '创建测品' }}</h1>
      </div>
      <div class="flex gap-2">
        <el-button @click="router.push(adminPath('/reservation-products'))">返回列表</el-button>
        <el-button @click="preview">预览</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </div>
    </header>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="space-y-5">
      <div class="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h2 class="mb-5 text-lg font-black text-slate-900">基础信息</h2>
        <div class="grid gap-4 md:grid-cols-2">
          <el-form-item label="Slug" prop="slug"><el-input v-model="form.slug" maxlength="120" placeholder="portable-solar-lamp" @blur="normalizeSlug" /></el-form-item>
          <el-form-item label="可选关联现有 SPU ID"><el-input-number v-model="form.linked_product_id" :min="1" :controls="false" class="w-full" placeholder="不关联可留空" /></el-form-item>
          <el-form-item label="状态"><el-select v-model="form.status" class="w-full"><el-option label="草稿" value="draft" /><el-option label="定时开放" value="scheduled" /><el-option label="开放" value="open" /><el-option label="关闭" value="closed" /><el-option label="归档" value="archived" /></el-select></el-form-item>
          <el-form-item label="展示排序"><el-input-number v-model="form.display_order" :min="-1000000" :max="1000000" class="w-full" /></el-form-item>
          <el-form-item label="开放开始时间"><input v-model="form.reservation_start_at" type="datetime-local" class="h-10 w-full rounded border border-slate-300 px-3 text-sm" /></el-form-item>
          <el-form-item label="开放结束时间"><input v-model="form.reservation_end_at" type="datetime-local" class="h-10 w-full rounded border border-slate-300 px-3 text-sm" /></el-form-item>
          <el-form-item label="热度覆盖"><el-select v-model="form.heat_level_override" clearable class="w-full"><el-option label="新品" value="new" /><el-option label="较受欢迎" value="popular" /><el-option label="热度上升" value="trending" /><el-option label="需求很高" value="highly_requested" /></el-select></el-form-item>
          <el-form-item label="推荐展示"><el-switch v-model="form.is_featured" active-text="推荐" inactive-text="普通" /></el-form-item>
        </div>
      </div>

      <div class="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <el-tabs v-model="activeLanguage">
          <el-tab-pane label="English" name="en">
            <el-form-item label="英文标题" prop="title_en"><el-input v-model="form.title_en" maxlength="200" show-word-limit /></el-form-item>
            <el-form-item label="英文简介"><el-input v-model="form.summary_en" type="textarea" :rows="3" maxlength="1000" show-word-limit /></el-form-item>
            <el-form-item label="英文详情"><RichTextDetailEditor v-model="form.description_en" /></el-form-item>
          </el-tab-pane>
          <el-tab-pane label="中文" name="zh">
            <el-form-item label="中文标题"><el-input v-model="form.title_zh" maxlength="200" show-word-limit /></el-form-item>
            <el-form-item label="中文简介"><el-input v-model="form.summary_zh" type="textarea" :rows="3" maxlength="1000" show-word-limit /></el-form-item>
            <el-form-item label="中文详情"><RichTextDetailEditor v-model="form.description_zh" /></el-form-item>
          </el-tab-pane>
        </el-tabs>
      </div>

      <div class="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h2 class="mb-5 text-lg font-black text-slate-900">安全图片</h2>
        <el-form-item label="主图">
          <el-upload v-model:file-list="mainImageFiles" :action="uploadAction" :headers="uploadHeaders" list-type="picture-card" :limit="1" accept="image/jpeg,image/png,image/webp" :before-upload="beforeImageUpload" :on-success="mainImageSuccess" :on-remove="removeMainImage">上传主图</el-upload>
        </el-form-item>
        <el-form-item label="画廊（最多 12 张）">
          <el-upload v-model:file-list="galleryFiles" :action="uploadAction" :headers="uploadHeaders" list-type="picture-card" :limit="12" multiple accept="image/jpeg,image/png,image/webp" :before-upload="beforeImageUpload" :on-success="gallerySuccess" :on-remove="galleryRemove">添加图片</el-upload>
        </el-form-item>
        <p class="text-xs text-slate-500">上传沿用现有安全图片服务。关联 SPU 时不会复制价格或库存字段。</p>
      </div>
    </el-form>

    <el-dialog v-model="previewOpen" title="前台预约页预览" width="min(900px, 94vw)" destroy-on-close>
      <div class="mb-4 flex gap-2"><el-button :type="previewLocale === 'en' ? 'primary' : 'default'" @click="previewLocale = 'en'">English</el-button><el-button :type="previewLocale === 'zh' ? 'primary' : 'default'" @click="previewLocale = 'zh'">中文</el-button></div>
      <div class="grid gap-6 md:grid-cols-2">
        <div class="aspect-square overflow-hidden rounded-2xl border border-slate-200"><ProductImage :src="form.main_image_url" :alt="previewTitle || 'Reservation product'" image-class="h-full w-full object-contain" /></div>
        <div>
          <ReservationHeatBadge :level="previewHeat" />
          <h2 class="mt-4 text-3xl font-black text-slate-950">{{ previewTitle || 'Untitled reservation product' }}</h2>
          <p class="mt-3 leading-7 text-slate-600">{{ previewSummary || 'No summary yet.' }}</p>
          <div class="mt-5 rounded-xl bg-slate-50 px-4 py-3 text-sm font-bold text-slate-600">状态：{{ form.status }} · {{ form.is_featured ? '推荐' : '普通' }}</div>
        </div>
      </div>
      <div class="mt-6 border-t border-slate-100 pt-5"><div v-if="previewDescription" class="leading-7 text-slate-700" v-html="previewDescription" /><p v-else class="text-sm text-slate-400">暂无详情</p></div>
    </el-dialog>
  </section>
</template>
