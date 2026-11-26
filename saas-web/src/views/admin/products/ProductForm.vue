<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules, UploadFile, UploadFiles, UploadProps, UploadRawFile, UploadUserFile } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  getAdminPlatformCategoryById,
  searchAdminPlatformCategories,
  type PlatformCategory,
} from '../../../api/platformCategories'
import { sanitizeProductDetailHtml } from '../../../utils/sanitizeProductDetail'
import { adminPath } from '../../../utils/adminPath.ts'
import RichTextDetailEditor from '../../../components/admin/RichTextDetailEditor.vue'
import {
  createAdminProduct,
  getAdminProductDetail,
  uploadAdminProductVideo,
  updateAdminProduct,
  type AdminProductPayload,
  type SKUSpecs,
  type SpecTemplateItem,
  type ProductStatus,
} from '../../../api/adminProducts'
import { useAuthStore } from '../../../stores/auth'
import { applySkuBatchToRows, hasSkuBatchValues, isValidBatchStock } from '../../../utils/skuBatch'
import { CANONICAL_DEFAULT_SKU_NAME, isDefaultSkuName } from '../../../utils/productSku'
import {
  isCurrentPlatformCategorySearch,
  shouldSchedulePlatformCategorySearch,
} from '../../../utils/platformCategorySearch.ts'

interface SKUForm {
  id?: number
  name: string
  erp_sku_code: string
  image_url: string
  image_files: UploadUserFile[]
  specs: SKUSpecs
  retail_price: number | null
  original_price: number | null
  sellable_stock: number | null
}

interface SpecForm {
  name: string
  values: string[]
}

interface ProductFormModel {
  platform_category_id: number | null
  name: string
  description: string
  detail_html: string
  main_image: string
  video_url: string
  material: string
  dimensions: string
  weight: string
  slider_images: string[]
  spec_template: SpecForm[]
  status: ProductStatus
  skus: SKUForm[]
}

interface ProductTranslationFormModel {
  name: string
  description: string
  detail_html: string
  material: string
  dimensions: string
  weight: string
}

interface SKUBatchForm {
  retail_price: number | null
  original_price: number | null
  sellable_stock: number | null
}

const route = useRoute()
const router = useRouter()
const { locale, t } = useI18n()
const authStore = useAuthStore()
const formRef = ref<FormInstance>()
const loading = ref(false)
const submitting = ref(false)
const platformCategorySearchResults = ref<PlatformCategory[]>([])
const platformCategorySearchLoading = ref(false)
const selectedPlatformCategory = ref<PlatformCategory | null>(null)
const platformCategoryPopoverVisible = ref(false)
const platformCategoryKeyword = ref('')
const platformCategoryInputComposing = ref(false)
let platformCategorySearchTimer: ReturnType<typeof setTimeout> | undefined
let platformCategorySearchAbortController: AbortController | undefined
let platformCategorySearchSequence = 0
const mainImageFiles = ref<UploadUserFile[]>([])
const sliderImageFiles = ref<UploadUserFile[]>([])
const videoUploading = ref(false)
const videoUploadProgress = ref(0)
const videoUploadIndeterminate = ref(false)
const videoUploadFileName = ref('')
const videoUploadFileSize = ref(0)
const videoUploadError = ref('')
const videoUploadSucceeded = ref(false)
const videoUploadStatusMessage = ref('')
const uploadAction = '/api/v1/admin/uploads/image'
const videoUploadAction = '/api/v1/admin/uploads/video'
const uploadHeaders = computed(() => ({
  ...(authStore.token ? { Authorization: `Bearer ${authStore.token}` } : {}),
  'Accept-Language': locale.value,
}))
const productId = computed(() => {
  const rawId = route.params.id
  return typeof rawId === 'string' ? Number(rawId) : undefined
})
const isEdit = computed(() => Number.isFinite(productId.value))

const form = reactive<ProductFormModel>({
  platform_category_id: null,
  name: '',
  description: '',
  detail_html: '',
  main_image: '',
  video_url: '',
  material: '',
  dimensions: '',
  weight: '',
  slider_images: [],
  spec_template: [],
  status: 'off_shelf',
  skus: [createEmptySku()],
})

const translationForm = reactive<ProductTranslationFormModel>({
  name: '',
  description: '',
  detail_html: '',
  material: '',
  dimensions: '',
  weight: '',
})

const skuBatchForm = reactive<SKUBatchForm>({
  retail_price: null,
  original_price: null,
  sellable_stock: null,
})

const skuCount = computed(() => form.skus.length)
const selectedPlatformCategoryLabel = computed(() =>
  selectedPlatformCategory.value ? platformCategorySelectedLabel(selectedPlatformCategory.value) : '',
)
const normalizedPlatformCategoryKeyword = computed(() => platformCategoryKeyword.value.trim().toLocaleLowerCase())
const matchingLeafPlatformCategoryCount = computed(
  () => platformCategorySearchResults.value.filter((category) => category.is_leaf).length,
)
const matchingParentPlatformCategoryCount = computed(
  () => platformCategorySearchResults.value.filter((category) => !category.is_leaf).length,
)
const videoFormatHelpText = computed(() =>
  locale.value === 'en'
    ? 'Supports MP4, WebM, MOV and M4V, up to 50MB'
    : '支持 MP4、WebM、MOV、M4V，最大 50MB',
)
const unsupportedVideoFormatText = computed(() =>
  locale.value === 'en'
    ? 'Unsupported video format. Please upload MP4, WebM, MOV or M4V video.'
    : '不支持的视频格式，请上传 MP4、WebM、MOV 或 M4V 视频',
)
const videoUploadFailedText = computed(() =>
  locale.value === 'en'
    ? 'Video upload failed. Please check the format and size, then try again.'
    : '视频上传失败，请检查格式和大小后重试',
)
const videoTooLargeText = computed(() =>
  locale.value === 'en'
    ? 'The video file is too large. Please upload a video under 50MB.'
    : '视频文件过大，请上传 50MB 以内的视频',
)
const imageTooLargeText = computed(() =>
  locale.value === 'en'
    ? 'Image file is too large. Please compress it and try again.'
    : '图片文件过大，请压缩后重试',
)
const videoUploadSuccessText = computed(() =>
  locale.value === 'en' ? 'Video uploaded successfully' : '视频上传成功',
)
const duplicateVideoReuseText = computed(() =>
  locale.value === 'en'
    ? 'Duplicate video detected. Existing uploaded video was reused.'
    : '检测到相同视频，已复用已上传视频',
)
const videoUploadingText = computed(() =>
  locale.value === 'en' ? 'Uploading...' : '上传中...',
)
const leafPlatformCategorySearchResults = computed(() =>
  platformCategorySearchResults.value.filter((category) => category.is_leaf),
)
const parentPlatformCategorySearchResults = computed(() =>
  platformCategorySearchResults.value.filter((category) => !category.is_leaf),
)
const platformCategorySearchPlaceholder = computed(() =>
  locale.value === 'en'
    ? 'Search English/Chinese category name or full path, choose the most specific category'
    : '搜索英文/中文类目名称或完整路径，选择最末级类目',
)
const platformCategorySearchHelpText = computed(() =>
  locale.value === 'en'
    ? 'Server-side search supports English and Chinese names or full paths; only leaf categories can be selected.'
    : '服务端搜索支持英文/中文类目名称或完整路径；仅最末级类目可选。',
)

const rules: FormRules<ProductFormModel> = {
  platform_category_id: [{ required: true, message: '请选择最末级类目', trigger: 'change' }],
  name: [{ required: true, message: '请输入商品名称', trigger: 'blur' }],
  main_image: [{ required: true, message: '请上传商品主图', trigger: 'change' }],
}

function platformCategoryDisplayPath(value: string) {
  return value.split('/').map((part) => part.trim()).filter(Boolean).join(' / ')
}

function platformCategoryEnglishPathLabel(category: PlatformCategory) {
  return platformCategoryDisplayPath(category.path)
}

function platformCategoryZhName(category: PlatformCategory) {
  return category.zh_name?.trim() || category.translations?.zh?.name?.trim() || category.name
}

function platformCategoryZhPathLabel(category: PlatformCategory) {
  return category.zh_path?.trim() || platformCategoryEnglishPathLabel(category)
}

function platformCategoryResultTitle(category: PlatformCategory) {
  const zhName = platformCategoryZhName(category)
  if (zhName && zhName !== category.name) {
    return `${zhName} / ${category.name}`
  }
  return category.name
}

function platformCategorySelectedLabel(category: PlatformCategory) {
  const zhPath = platformCategoryZhPathLabel(category)
  const englishPath = platformCategoryEnglishPathLabel(category)
  return zhPath === englishPath ? englishPath : `${zhPath} / ${englishPath}`
}

function showMostSpecificCategoryMessage() {
  ElMessage.warning('请选择最末级类目 / Please select the most specific category')
}

function selectPlatformCategory(category: PlatformCategory) {
  if (!category.is_leaf) {
    showMostSpecificCategoryMessage()
    return
  }
  form.platform_category_id = category.id
  selectedPlatformCategory.value = category
  clearPlatformCategorySearch()
  platformCategoryPopoverVisible.value = false
  void formRef.value?.validateField('platform_category_id')
}

function handlePlatformCategoryInput(value: string) {
  platformCategoryKeyword.value = value
  platformCategoryPopoverVisible.value = true
  if (platformCategorySearchTimer) {
    clearTimeout(platformCategorySearchTimer)
    platformCategorySearchTimer = undefined
  }
  platformCategorySearchAbortController?.abort()
  platformCategorySearchAbortController = undefined
  const sequence = ++platformCategorySearchSequence
  if (!shouldSchedulePlatformCategorySearch(value, platformCategoryInputComposing.value)) {
    platformCategorySearchLoading.value = false
    if (!platformCategoryInputComposing.value) {
      platformCategorySearchResults.value = []
    }
    return
  }
  const keyword = value.trim()
  platformCategorySearchTimer = setTimeout(() => {
    void searchPlatformCategoryOptions(keyword, sequence)
  }, 250)
}

async function searchPlatformCategoryOptions(keyword: string, sequence: number) {
  const controller = new AbortController()
  platformCategorySearchAbortController = controller
  platformCategorySearchLoading.value = true
  try {
    const results = await searchAdminPlatformCategories(keyword, {
      limit: 50,
      locale: 'all',
      leafOnly: false,
      signal: controller.signal,
    })
    if (isCurrentPlatformCategorySearch(sequence, platformCategorySearchSequence, keyword, platformCategoryKeyword.value)) {
      platformCategorySearchResults.value = results
    }
  } catch (error) {
    if (sequence === platformCategorySearchSequence) {
      ElMessage.error(error instanceof Error ? error.message : '平台类目搜索失败')
    }
  } finally {
    if (sequence === platformCategorySearchSequence) {
      platformCategorySearchLoading.value = false
    }
    if (platformCategorySearchAbortController === controller) {
      platformCategorySearchAbortController = undefined
    }
  }
}

function handlePlatformCategoryCompositionStart() {
  platformCategoryInputComposing.value = true
  if (platformCategorySearchTimer) {
    clearTimeout(platformCategorySearchTimer)
    platformCategorySearchTimer = undefined
  }
  platformCategorySearchAbortController?.abort()
  platformCategorySearchAbortController = undefined
  platformCategorySearchSequence += 1
  platformCategorySearchLoading.value = false
}

function handlePlatformCategoryCompositionEnd(event: CompositionEvent) {
  platformCategoryInputComposing.value = false
  const input = event.target as HTMLInputElement | null
  handlePlatformCategoryInput(input?.value ?? platformCategoryKeyword.value)
}

function clearPlatformCategorySearch() {
  platformCategoryInputComposing.value = false
  platformCategoryKeyword.value = ''
  platformCategorySearchResults.value = []
  platformCategorySearchSequence += 1
  platformCategorySearchLoading.value = false
  platformCategorySearchAbortController?.abort()
  platformCategorySearchAbortController = undefined
  if (platformCategorySearchTimer) {
    clearTimeout(platformCategorySearchTimer)
    platformCategorySearchTimer = undefined
  }
}

function clearPlatformCategorySelection() {
  form.platform_category_id = null
  selectedPlatformCategory.value = null
  clearPlatformCategorySearch()
  void formRef.value?.validateField('platform_category_id')
}

function handlePlatformCategoryPopoverHide() {
  clearPlatformCategorySearch()
}

function validatePlatformCategory() {
  const category = selectedPlatformCategory.value
  if (!category?.is_leaf) {
    ElMessage.error('请选择最末级类目后再保存商品')
    return false
  }
  return true
}

onMounted(async () => {
  loading.value = true
  try {
    if (isEdit.value && productId.value) {
      const product = await getAdminProductDetail(productId.value)
      form.platform_category_id = product.platform_category_id
      selectedPlatformCategory.value = await getAdminPlatformCategoryById(product.platform_category_id)
      form.name = product.name
      form.description = product.description
      form.detail_html = product.detail_html || ''
      form.main_image = product.main_image
      form.video_url = product.video_url || ''
      form.material = product.material || ''
      form.dimensions = product.dimensions || ''
      form.weight = product.weight || ''
      applyProductTranslations(product.translations?.zh)
      form.slider_images = parseSliderImages(product.slider_images)
      form.spec_template = parseSpecTemplate(product.spec_template)
      mainImageFiles.value = product.main_image ? [urlToUploadFile(product.main_image)] : []
      sliderImageFiles.value = form.slider_images.map((url) => urlToUploadFile(url))
      form.status = product.status
      form.skus = product.skus.length
        ? product.skus.map((sku) => ({
            id: sku.id,
            name: sku.name,
            erp_sku_code: sku.erp_sku_code,
            image_url: sku.image_url || '',
            image_files: sku.image_url ? [urlToUploadFile(sku.image_url)] : [],
            specs: { ...(sku.specs || {}) },
            retail_price: centsToDecimal(sku.retail_price),
            original_price: centsToDecimal(sku.original_price),
            sellable_stock: sku.sellable_stock,
          }))
        : [createEmptySku()]
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '表单数据加载失败')
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  if (platformCategorySearchTimer) {
    clearTimeout(platformCategorySearchTimer)
  }
  platformCategorySearchAbortController?.abort()
  platformCategorySearchSequence += 1
})

function createEmptySku(): SKUForm {
  return {
    id: undefined,
    name: '',
    erp_sku_code: '',
    image_url: '',
    image_files: [],
    specs: {},
    retail_price: null,
    original_price: null,
    sellable_stock: null,
  }
}

function applyProductTranslations(translations?: Partial<ProductTranslationFormModel>) {
  Object.assign(translationForm, {
    name: translations?.name || '',
    description: translations?.description || '',
    detail_html: translations?.detail_html || '',
    material: translations?.material || '',
    dimensions: translations?.dimensions || '',
    weight: translations?.weight || '',
  })
}

function centsToDecimal(cents: number) {
  return Number((cents / 100).toFixed(2))
}

function decimalToCents(value: number) {
  return Math.round(value * 100)
}

function parseSliderImages(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === 'string')
  }

  if (typeof value === 'string' && value) {
    try {
      const parsed = JSON.parse(value) as unknown
      return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : []
    } catch {
      return []
    }
  }

  return []
}

function parseSpecTemplate(value: unknown): SpecForm[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value
    .map((item) => {
      if (!item || typeof item !== 'object') {
        return null
      }
      const candidate = item as Partial<SpecTemplateItem>
      const name = typeof candidate.name === 'string' ? candidate.name : ''
      const values = Array.isArray(candidate.values) ? candidate.values.filter((entry): entry is string => typeof entry === 'string') : []
      return { name, values }
    })
    .filter((item): item is SpecForm => Boolean(item))
    .slice(0, 3)
}

function urlToUploadFile(url: string): UploadUserFile {
  return {
    name: url.split('/').pop() || url,
    url,
  }
}

function extractUploadURL(response: unknown) {
  const uploadResponse = response as { data?: { url?: string } }
  return uploadResponse.data?.url || ''
}

function isPersistableImageURL(url: string) {
  const value = url.trim()
  if (!value || value.startsWith('blob:') || value.startsWith('file:') || /^[a-zA-Z]:\\/.test(value) || value.includes('\\')) {
    return false
  }
  return value.startsWith('/uploads/') || /^https?:\/\//i.test(value)
}

function isPersistableVideoURL(url: string) {
  const value = url.trim()
  if (!value || value.startsWith('blob:') || value.startsWith('file:') || /^[a-zA-Z]:\\/.test(value) || value.includes('\\')) {
    return false
  }
  return value.startsWith('/uploads/videos/') || /^https?:\/\//i.test(value)
}

function validateImageURLs() {
  if (!isPersistableImageURL(form.main_image)) {
    ElMessage.error('请上传商品主图后再保存')
    return false
  }
  const invalidSliderImage = form.slider_images.find((url) => !isPersistableImageURL(url))
  if (invalidSliderImage) {
    ElMessage.error('轮播图包含不可保存的本地路径，请重新上传')
    return false
  }
  const invalidSKUImageIndex = form.skus.findIndex((sku) => sku.image_url && !isPersistableImageURL(sku.image_url))
  if (invalidSKUImageIndex >= 0) {
    ElMessage.error(`第 ${invalidSKUImageIndex + 1} 行 SKU 图片不可保存，请重新上传`)
    return false
  }
  if (form.video_url && !isPersistableVideoURL(form.video_url)) {
    ElMessage.error('商品视频地址无效，请重新上传或清空')
    return false
  }
  return true
}

const beforeImageUpload: UploadProps['beforeUpload'] = (rawFile: UploadRawFile) => {
  const allowedTypes = ['image/jpeg', 'image/png', 'image/webp']
  if (!allowedTypes.includes(rawFile.type)) {
    ElMessage.error('仅支持 JPG、PNG、WEBP 图片')
    return false
  }
  if (rawFile.size > 5 * 1024 * 1024) {
    ElMessage.error('图片大小不能超过 5MB')
    return false
  }
  return true
}

const beforeVideoUpload: UploadProps['beforeUpload'] = (rawFile: UploadRawFile) => {
  if (videoUploading.value) {
    return false
  }
  const extension = rawFile.name.toLowerCase().split('.').pop()
  const contentType = rawFile.type.split(';', 1)[0]?.trim().toLowerCase() || ''
  const allowedTypes = ['', 'application/octet-stream', 'video/mp4', 'video/webm', 'video/quicktime', 'video/x-m4v']
  const allowedExtensions = ['mp4', 'webm', 'mov', 'm4v']
  if (!extension || !allowedExtensions.includes(extension) || !allowedTypes.includes(contentType)) {
    ElMessage.error(unsupportedVideoFormatText.value)
    return false
  }
  if (rawFile.size > 50 * 1024 * 1024) {
    ElMessage.error(videoTooLargeText.value)
    return false
  }
  return true
}

const uploadProductVideoRequest: UploadProps['httpRequest'] = async (options) => {
  videoUploading.value = true
  videoUploadProgress.value = 0
  videoUploadIndeterminate.value = false
  videoUploadFileName.value = options.file.name
  videoUploadFileSize.value = options.file.size
  videoUploadError.value = ''
  videoUploadSucceeded.value = false
  videoUploadStatusMessage.value = ''

  try {
    const result = await uploadAdminProductVideo(options.file, locale.value, (percentage) => {
      if (percentage === null) {
        videoUploadIndeterminate.value = true
        return
      }
      videoUploadIndeterminate.value = false
      videoUploadProgress.value = percentage
    })
    if (!isPersistableVideoURL(result.url)) {
      throw new Error('Video upload returned an invalid URL')
    }

    form.video_url = result.url
    videoUploadProgress.value = 100
    videoUploadIndeterminate.value = false
    videoUploadSucceeded.value = true
    videoUploadStatusMessage.value = result.duplicated ? duplicateVideoReuseText.value : videoUploadSuccessText.value
    ElMessage.success(videoUploadStatusMessage.value)
    return { data: result }
  } catch (error) {
    const message = videoUploadErrorMessage(error)
    videoUploadError.value = message
    videoUploadSucceeded.value = false
    ElMessage.error(message)
    throw error instanceof Error ? error : new Error(message)
  } finally {
    videoUploading.value = false
  }
}

function clearProductVideo() {
  form.video_url = ''
  videoUploadProgress.value = 0
  videoUploadIndeterminate.value = false
  videoUploadFileName.value = ''
  videoUploadFileSize.value = 0
  videoUploadError.value = ''
  videoUploadSucceeded.value = false
  videoUploadStatusMessage.value = ''
}

function formatVideoFileSize(bytes: number) {
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function videoUploadErrorMessage(error: unknown) {
  const message = error instanceof Error ? error.message : ''
  if (/413|request entity too large|content too large|上传文件超过大小限制/i.test(message)) {
    return videoTooLargeText.value
  }
  return videoUploadFailedText.value
}

const handleMainImageSuccess: UploadProps['onSuccess'] = (response: unknown, uploadFile: UploadFile) => {
  const url = extractUploadURL(response)
  if (!url || !isPersistableImageURL(url)) {
    ElMessage.error('主图上传失败')
    return
  }

  form.main_image = url
  mainImageFiles.value = [{ name: uploadFile.name, url }]
  void formRef.value?.validateField('main_image')
}

const handleSliderImageSuccess: UploadProps['onSuccess'] = (response: unknown, uploadFile: UploadFile) => {
  const url = extractUploadURL(response)
  if (!url || !isPersistableImageURL(url)) {
    ElMessage.error('轮播图上传失败')
    return
  }

  uploadFile.url = url
  if (sliderImageFiles.value.some((file) => file.uid === uploadFile.uid)) {
    sliderImageFiles.value = sliderImageFiles.value.map((file) => (file.uid === uploadFile.uid ? { ...file, name: uploadFile.name, url } : file))
  } else {
    sliderImageFiles.value = [...sliderImageFiles.value, { name: uploadFile.name, url, uid: uploadFile.uid }]
  }
  syncSliderImages()
}

function skuImageSuccessHandler(row: SKUForm): UploadProps['onSuccess'] {
  return (response: unknown, uploadFile: UploadFile) => {
    const url = extractUploadURL(response)
    if (!url || !isPersistableImageURL(url)) {
      ElMessage.error('SKU 图片上传失败')
      return
    }

    row.image_url = url
    row.image_files = [{ name: uploadFile.name, url, uid: uploadFile.uid }]
  }
}

function skuImageRemoveHandler(row: SKUForm): UploadProps['onRemove'] {
  return () => {
    row.image_url = ''
    row.image_files = []
  }
}

const handleSKUImageExceed: UploadProps['onExceed'] = () => {
  ElMessage.warning('每个 SKU 只能上传 1 张图片，请先移除当前图片')
}

const handleMainImageRemove: UploadProps['onRemove'] = () => {
  form.main_image = ''
  mainImageFiles.value = []
  void formRef.value?.validateField('main_image')
}

const handleSliderImageRemove: UploadProps['onRemove'] = (_uploadFile: UploadFile, uploadFiles: UploadFiles) => {
  sliderImageFiles.value = uploadFiles
  syncSliderImages()
}

const handleUploadError: UploadProps['onError'] = (error) => {
  ElMessage.error(uploadErrorMessage(error))
}

const handleMainImageExceed: UploadProps['onExceed'] = () => {
  ElMessage.warning('商品主图只能上传 1 张，请先移除当前图片')
}

function syncSliderImages() {
  form.slider_images = sliderImageFiles.value.map((file) => file.url).filter((url): url is string => Boolean(url && isPersistableImageURL(url)))
}

function uploadErrorMessage(error: Error) {
  if (!error.message) {
    return '图片上传失败，请重试'
  }
  try {
    const parsed = JSON.parse(error.message) as { error?: string }
    if (parsed.error) {
      return parsed.error
    }
  } catch {
    // Element Plus may pass plain network errors here.
  }
  if (/413|request entity too large|content too large/i.test(error.message)) {
    return imageTooLargeText.value
  }
  return error.message || '图片上传失败，请重试'
}

function addSpec() {
  if (form.spec_template.length >= 3) {
    ElMessage.warning('最多添加 3 组规格')
    return
  }

  form.spec_template.push({ name: '', values: [''] })
}

function removeSpec(index: number) {
  form.spec_template.splice(index, 1)
  regenerateSkus()
}

function addSpecValue(spec: SpecForm) {
  spec.values.push('')
}

function removeSpecValue(spec: SpecForm, valueIndex: number) {
  spec.values.splice(valueIndex, 1)
  if (!spec.values.length) {
    spec.values.push('')
  }
  regenerateSkus()
}

function normalizedSpecTemplate(): SpecTemplateItem[] {
  return form.spec_template
    .map((spec) => ({
      name: spec.name.trim(),
      values: uniqueValues(spec.values.map((value) => value.trim()).filter(Boolean)),
    }))
    .filter((spec) => spec.name && spec.values.length)
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values))
}

function cartesianProduct(specs: SpecTemplateItem[]): SKUSpecs[] {
  if (!specs.length) {
    return [{}]
  }

  return specs.reduce<SKUSpecs[]>(
    (combinations, spec) =>
      combinations.flatMap((combination) =>
        spec.values.map((value) => ({
          ...combination,
          [spec.name]: value,
        })),
      ),
    [{}],
  )
}

function specKey(specs: SKUSpecs) {
  return Object.keys(specs)
    .sort()
    .map((key) => `${key}:${specs[key]}`)
    .join('|')
}

function skuDisplayName(specs: SKUSpecs) {
  const values = Object.values(specs).filter(Boolean)
  return values.length ? values.join(' / ') : CANONICAL_DEFAULT_SKU_NAME
}

function skuNameForSave(sku: SKUForm) {
  const name = sku.name.trim()
  if (!Object.keys(sku.specs).length && (!name || isDefaultSkuName(name))) {
    return CANONICAL_DEFAULT_SKU_NAME
  }
  return name || skuDisplayName(sku.specs)
}

function skuCodeDisplay(row: SKUForm) {
  return row.erp_sku_code || '保存后生成 SKU 编码'
}

function regenerateSkus() {
  const specs = normalizedSpecTemplate()
  if (!specs.length) {
    if (!form.skus.length) {
      form.skus = [createEmptySku()]
    }
    form.skus = form.skus.slice(0, 1).map((sku) => ({
      ...sku,
      name: !sku.name || isDefaultSkuName(sku.name) ? CANONICAL_DEFAULT_SKU_NAME : sku.name,
      specs: {},
    }))
    return
  }

  const existingBySpec = new Map(form.skus.map((sku) => [specKey(sku.specs), sku]))
  const combinations = cartesianProduct(specs)
  form.skus = combinations.map((specValues, index) => {
    const existing = existingBySpec.get(specKey(specValues))
    const name = skuDisplayName(specValues)
    return {
      id: existing?.id,
      name,
      erp_sku_code: existing?.erp_sku_code || '',
      image_url: existing?.image_url || '',
      image_files: existing?.image_files || [],
      specs: specValues,
      retail_price: existing?.retail_price ?? form.skus[0]?.retail_price ?? null,
      original_price: existing?.original_price ?? form.skus[0]?.original_price ?? null,
      sellable_stock: existing?.sellable_stock ?? (index === 0 ? form.skus[0]?.sellable_stock ?? null : null),
    }
  })
}

function validateSkus() {
  if (!form.skus.length) {
    ElMessage.error('请至少添加一个 SKU')
    return false
  }

  for (const [index, sku] of form.skus.entries()) {
    const rowNumber = index + 1
    if (sku.retail_price === null || sku.retail_price <= 0) {
      ElMessage.error(`第 ${rowNumber} 行 SKU 零售价必须大于 0`)
      return false
    }
    if (sku.original_price === null || sku.original_price < 0) {
      ElMessage.error(`第 ${rowNumber} 行原价必填且不能为负数`)
      return false
    }
    if (sku.sellable_stock === null || sku.sellable_stock < 0 || !Number.isInteger(sku.sellable_stock)) {
      ElMessage.error(`第 ${rowNumber} 行库存必须是非负整数`)
      return false
    }
  }

  return true
}

function applySkuBatchValues() {
  if (!hasSkuBatchValues(skuBatchForm)) {
    ElMessage.warning('请至少填写一个批量设置项')
    return
  }
  if (!isValidBatchStock(skuBatchForm.sellable_stock)) {
    ElMessage.error('库存必须是非负整数')
    return
  }

  form.skus = applySkuBatchToRows(form.skus, skuBatchForm)
  ElMessage.success('已应用到全部 SKU')
}

function buildPayload(): AdminProductPayload {
  return {
    platform_category_id: form.platform_category_id ?? 0,
    name: form.name.trim(),
    description: form.description.trim(),
    detail_html: sanitizeProductDetailHtml(form.detail_html),
    main_image: form.main_image.trim(),
    video_url: form.video_url.trim(),
    material: form.material.trim(),
    dimensions: form.dimensions.trim(),
    weight: form.weight.trim(),
    slider_images: form.slider_images,
    spec_template: normalizedSpecTemplate(),
    status: form.status,
    translations: {
      zh: {
        name: translationForm.name.trim(),
        description: translationForm.description.trim(),
        detail_html: sanitizeProductDetailHtml(translationForm.detail_html),
        material: translationForm.material.trim(),
        dimensions: translationForm.dimensions.trim(),
        weight: translationForm.weight.trim(),
      },
    },
    skus: form.skus.map((sku) => ({
      id: sku.id,
      name: skuNameForSave(sku),
      erp_sku_code: sku.id ? sku.erp_sku_code.trim() : '',
      image_url: sku.image_url.trim(),
      specs: sku.specs,
      retail_price: decimalToCents(sku.retail_price ?? 0),
      original_price: decimalToCents(sku.original_price ?? 0),
      sellable_stock: sku.sellable_stock ?? 0,
    })),
  }
}

async function submitForm() {
  if (!formRef.value) {
    return
  }
  if (videoUploading.value) {
    ElMessage.warning(videoUploadingText.value)
    return
  }

  form.detail_html = sanitizeProductDetailHtml(form.detail_html)
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid || !validatePlatformCategory() || !validateImageURLs() || !validateSkus()) {
    return
  }

  submitting.value = true
  try {
    const payload = buildPayload()
    if (isEdit.value && productId.value) {
      await updateAdminProduct(productId.value, payload)
      ElMessage.success('商品已更新')
    } else {
      await createAdminProduct(payload)
      ElMessage.success('商品已创建')
    }
    await router.push(adminPath('/products'))
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section v-loading="loading" class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">{{ isEdit ? '编辑商品' : '新增商品' }}</h2>
        <p class="mt-1 text-sm text-slate-500">维护 SPU 基础信息与 SKU 价格库存</p>
      </div>
      <el-button @click="router.push(adminPath('/products'))">返回列表</el-button>
    </div>

    <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" class="space-y-4" @submit.prevent="submitForm">
      <el-card shadow="never">
        <template #header>SPU 基础信息</template>

        <el-tabs class="admin-translation-tabs">
          <el-tab-pane :label="t('adminTranslations.english')">
            <el-form-item label="商品名称" prop="name">
              <el-input v-model="form.name" placeholder="请输入商品名称" />
            </el-form-item>

            <el-form-item label="商品描述">
              <el-input v-model="form.description" type="textarea" :rows="4" placeholder="请输入商品简介；长图文内容请填写下方商品详情" />
            </el-form-item>

            <div class="grid gap-x-4 md:grid-cols-3">
              <el-form-item label="材质（可选）">
                <el-input v-model="form.material" maxlength="255" placeholder="例如：Cotton / 棉" />
              </el-form-item>
              <el-form-item label="尺寸（可选）">
                <el-input v-model="form.dimensions" maxlength="255" placeholder="例如：30 × 20 × 10 cm" />
              </el-form-item>
              <el-form-item label="重量（可选）">
                <el-input v-model="form.weight" maxlength="255" placeholder="例如：1.2 kg" />
              </el-form-item>
            </div>
          </el-tab-pane>

          <el-tab-pane :label="t('adminTranslations.chinese')">
            <el-form-item label="中文商品名称">
              <el-input v-model="translationForm.name" maxlength="200" placeholder="可选；不填则前台 fallback" />
            </el-form-item>

            <el-form-item label="中文商品描述">
              <el-input v-model="translationForm.description" type="textarea" :rows="4" maxlength="2000" show-word-limit placeholder="可选；不填则前台 fallback" />
            </el-form-item>

            <div class="grid gap-x-4 md:grid-cols-3">
              <el-form-item label="中文材质">
                <el-input v-model="translationForm.material" maxlength="255" placeholder="可选" />
              </el-form-item>
              <el-form-item label="中文尺寸">
                <el-input v-model="translationForm.dimensions" maxlength="255" placeholder="可选" />
              </el-form-item>
              <el-form-item label="中文重量">
                <el-input v-model="translationForm.weight" maxlength="255" placeholder="可选" />
              </el-form-item>
            </div>
          </el-tab-pane>
        </el-tabs>

        <el-form-item label="平台类目" prop="platform_category_id">
          <el-popover
            v-model:visible="platformCategoryPopoverVisible"
            placement="bottom-start"
            trigger="click"
            :width="640"
            popper-class="product-platform-category-popper"
            @hide="handlePlatformCategoryPopoverHide"
          >
            <template #reference>
              <el-input
                :model-value="platformCategoryPopoverVisible ? platformCategoryKeyword : selectedPlatformCategoryLabel"
                class="w-full"
                :title="selectedPlatformCategoryLabel"
                :placeholder="platformCategorySearchPlaceholder"
                @focus="platformCategoryPopoverVisible = true"
                @input="handlePlatformCategoryInput"
                @compositionstart="handlePlatformCategoryCompositionStart"
                @compositionend="handlePlatformCategoryCompositionEnd"
              />
            </template>

            <div class="space-y-3">
              <div class="flex items-center justify-between gap-3 border-b border-slate-100 pb-2">
                <p class="text-sm font-bold text-slate-800">
                  {{ normalizedPlatformCategoryKeyword ? `搜索“${platformCategoryKeyword.trim()}”` : '搜索平台类目' }}
                </p>
                <div class="flex items-center gap-2">
                  <el-button v-if="normalizedPlatformCategoryKeyword" link size="small" @click="clearPlatformCategorySearch">清空搜索</el-button>
                  <el-button v-if="selectedPlatformCategory" link type="danger" size="small" @click="clearPlatformCategorySelection">清除已选</el-button>
                  <p class="text-xs text-slate-400">仅最末级类目可选</p>
                </div>
              </div>

              <div v-if="normalizedPlatformCategoryKeyword" v-loading="platformCategorySearchLoading" class="min-h-24 max-h-80 overflow-y-auto pr-1">
                <template v-if="platformCategorySearchResults.length">
                  <div v-if="leafPlatformCategorySearchResults.length" class="space-y-1">
                    <p class="sticky top-0 z-10 bg-white py-1 text-xs font-bold text-emerald-700">
                      可选择的最末级类目（{{ matchingLeafPlatformCategoryCount }}）
                    </p>
                    <button
                      v-for="category in leafPlatformCategorySearchResults"
                      :key="category.id"
                      type="button"
                      class="flex w-full items-start justify-between gap-3 rounded-lg px-3 py-2 text-left hover:bg-emerald-50"
                      @click="selectPlatformCategory(category)"
                    >
                      <span class="min-w-0 space-y-0.5">
                        <span class="block truncate text-sm font-semibold text-slate-800">{{ platformCategoryResultTitle(category) }}</span>
                        <span class="block truncate text-xs text-slate-600">{{ platformCategoryZhPathLabel(category) }}</span>
                        <span class="block truncate text-xs text-slate-400">{{ platformCategoryEnglishPathLabel(category) }}</span>
                      </span>
                      <el-tag type="success" size="small">可选择</el-tag>
                    </button>
                  </div>

                  <div v-if="parentPlatformCategorySearchResults.length" class="mt-3 space-y-1 border-t border-slate-100 pt-2">
                    <p class="sticky top-0 z-10 bg-white py-1 text-xs font-bold text-amber-700">
                      父级类目（不可选择，共 {{ matchingParentPlatformCategoryCount }} 个）
                    </p>
                    <button
                      v-for="category in parentPlatformCategorySearchResults"
                      :key="category.id"
                      type="button"
                      disabled
                      class="flex w-full cursor-not-allowed items-start justify-between gap-3 rounded-lg px-3 py-2 text-left opacity-75"
                    >
                      <span class="min-w-0 space-y-0.5">
                        <span class="block truncate text-sm font-semibold text-slate-600">{{ platformCategoryResultTitle(category) }}</span>
                        <span class="block truncate text-xs text-slate-500">{{ platformCategoryZhPathLabel(category) }}</span>
                        <span class="block truncate text-xs text-slate-400">{{ platformCategoryEnglishPathLabel(category) }}</span>
                      </span>
                      <span class="shrink-0 text-xs text-amber-700">请选择最末级类目</span>
                    </button>
                  </div>
                </template>
                <el-empty v-else-if="!platformCategorySearchLoading" description="未找到匹配的平台类目" :image-size="72" />
              </div>

              <div v-else class="space-y-3">
                <div v-if="selectedPlatformCategory" class="rounded-lg border border-emerald-100 bg-emerald-50 px-3 py-2">
                  <p class="truncate text-sm font-semibold text-emerald-900">{{ platformCategoryResultTitle(selectedPlatformCategory) }}</p>
                  <p class="mt-1 truncate text-xs text-emerald-800">{{ platformCategoryZhPathLabel(selectedPlatformCategory) }}</p>
                  <p class="mt-0.5 truncate text-xs text-emerald-700">{{ platformCategoryEnglishPathLabel(selectedPlatformCategory) }}</p>
                </div>
                <div class="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-3 py-4 text-sm text-slate-500">
                  {{ platformCategorySearchHelpText }}
                </div>
              </div>
            </div>
          </el-popover>
          <div v-if="selectedPlatformCategory" class="mt-2 rounded-lg bg-slate-50 px-3 py-2">
            <p class="truncate text-xs font-semibold text-slate-700">{{ platformCategoryZhPathLabel(selectedPlatformCategory) }}</p>
            <p class="mt-0.5 truncate text-xs text-slate-500">{{ platformCategoryEnglishPathLabel(selectedPlatformCategory) }}</p>
          </div>
          <p class="mt-1 text-xs text-slate-500">{{ platformCategorySearchHelpText }}</p>
        </el-form-item>

        <el-form-item label="状态" prop="status">
          <el-radio-group v-model="form.status">
            <el-radio-button label="off_shelf">下架</el-radio-button>
            <el-radio-button label="on_shelf">上架</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="商品主图" prop="main_image">
          <el-upload
            v-model:file-list="mainImageFiles"
            :action="uploadAction"
            :headers="uploadHeaders"
            list-type="picture-card"
            accept="image/jpeg,image/png,image/webp"
            name="file"
            :limit="1"
            :before-upload="beforeImageUpload"
            :on-success="handleMainImageSuccess"
            :on-remove="handleMainImageRemove"
            :on-error="handleUploadError"
            :on-exceed="handleMainImageExceed"
          >
            <el-icon><Plus /></el-icon>
          </el-upload>
        </el-form-item>

        <el-form-item label="商品视频（可选）">
          <div class="w-full space-y-3">
            <p class="text-xs text-slate-500">{{ videoFormatHelpText }}</p>
            <div v-if="form.video_url" class="max-w-xl overflow-hidden rounded-xl border border-slate-200 bg-black">
              <video
                :src="form.video_url"
                class="aspect-video w-full object-contain"
                controls
                preload="metadata"
                playsinline
              />
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <el-upload
                :action="videoUploadAction"
                accept="video/mp4,video/webm,video/quicktime,video/x-m4v,.mp4,.webm,.mov,.m4v"
                name="file"
                :show-file-list="false"
                :disabled="videoUploading"
                :before-upload="beforeVideoUpload"
                :http-request="uploadProductVideoRequest"
              >
                <el-button type="primary" plain :loading="videoUploading" :disabled="videoUploading">
                  {{ videoUploading ? videoUploadingText : form.video_url ? '替换视频' : '上传视频' }}
                </el-button>
              </el-upload>
              <el-button v-if="form.video_url" type="danger" plain :disabled="videoUploading" @click="clearProductVideo">清空视频</el-button>
            </div>

            <div
              v-if="videoUploadFileName"
              class="max-w-xl space-y-2 rounded-xl border border-slate-200 bg-slate-50 p-3"
            >
              <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
                <span class="min-w-0 truncate font-semibold text-slate-700">{{ videoUploadFileName }}</span>
                <span class="shrink-0 text-slate-500">{{ formatVideoFileSize(videoUploadFileSize) }}</span>
              </div>
              <el-progress
                v-if="videoUploading || videoUploadSucceeded"
                :percentage="videoUploadProgress"
                :indeterminate="videoUploadIndeterminate"
                :show-text="!videoUploadIndeterminate"
                :status="videoUploadSucceeded ? 'success' : undefined"
              />
              <p v-if="videoUploading && videoUploadIndeterminate" class="text-xs text-blue-600">{{ videoUploadingText }}</p>
              <p v-else-if="videoUploading" class="text-xs text-blue-600">{{ videoUploadingText }} {{ videoUploadProgress }}%</p>
              <p v-else-if="videoUploadSucceeded" class="text-xs font-semibold text-emerald-600">{{ videoUploadStatusMessage }}</p>
              <p v-if="videoUploadError" class="text-xs font-semibold text-red-600">{{ videoUploadError }}</p>
            </div>
          </div>
        </el-form-item>

        <el-form-item label="轮播图">
          <el-upload
            v-model:file-list="sliderImageFiles"
            :action="uploadAction"
            :headers="uploadHeaders"
            list-type="picture-card"
            accept="image/jpeg,image/png,image/webp"
            name="file"
            multiple
            :before-upload="beforeImageUpload"
            :on-success="handleSliderImageSuccess"
            :on-remove="handleSliderImageRemove"
            :on-error="handleUploadError"
          >
            <el-icon><Plus /></el-icon>
          </el-upload>
        </el-form-item>

      </el-card>

      <el-card shadow="never">
        <template #header>
          <div>
            <span>商品详情</span>
            <p class="mt-1 text-xs font-normal text-slate-500">支持粘贴图文、上传长图或直接编辑 HTML 源码，内容在编辑框内部滚动。</p>
          </div>
        </template>

        <el-tabs class="admin-translation-tabs">
          <el-tab-pane :label="t('adminTranslations.english')">
            <RichTextDetailEditor v-model="form.detail_html" />
          </el-tab-pane>
          <el-tab-pane :label="t('adminTranslations.chinese')">
            <RichTextDetailEditor v-model="translationForm.detail_html" />
          </el-tab-pane>
        </el-tabs>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="flex items-center justify-between">
            <div>
              <span>商品规格</span>
            </div>
            <el-button type="primary" plain :disabled="form.spec_template.length >= 3" @click="addSpec">添加规格</el-button>
          </div>
        </template>

        <div class="space-y-4">
          <p class="text-sm text-slate-500">最多 3 组规格。规格名和值变化后会自动生成 SKU 组合，已有组合的价格和库存会保留。</p>
          <div v-if="!form.spec_template.length" class="rounded-2xl border border-dashed border-slate-200 p-4 text-sm text-slate-500">
            未配置规格时会保留一个默认 SKU。
          </div>

          <div v-for="(spec, specIndex) in form.spec_template" :key="specIndex" class="rounded-2xl border border-slate-200 p-4">
            <div class="grid gap-3 lg:grid-cols-[240px_1fr_auto]">
              <el-input v-model="spec.name" placeholder="规格名，如颜色" @change="regenerateSkus" />
              <div class="flex flex-wrap gap-2">
                <div v-for="(_value, valueIndex) in spec.values" :key="valueIndex" class="flex items-center gap-1">
                  <el-input v-model="spec.values[valueIndex]" class="w-36" placeholder="规格值" @change="regenerateSkus" />
                  <el-button text type="danger" @click="removeSpecValue(spec, valueIndex)">删除</el-button>
                </div>
                <el-button plain @click="addSpecValue(spec)">添加值</el-button>
              </div>
              <el-button type="danger" plain @click="removeSpec(specIndex)">删除规格</el-button>
            </div>
          </div>
        </div>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <div class="flex items-center justify-between">
            <span>SKU 列表</span>
            <span class="text-sm text-slate-500">{{ skuCount }} 个组合</span>
          </div>
        </template>

        <div class="mb-4 rounded-lg border border-slate-200 bg-slate-50 p-4">
          <div class="grid gap-3 lg:grid-cols-[1fr_1fr_1fr_auto] lg:items-end">
            <el-form-item label="批量零售价（GHS）" class="mb-0">
              <el-input-number v-model="skuBatchForm.retail_price" class="w-full" :min="0" :precision="2" :step="0.01" controls-position="right" />
            </el-form-item>
            <el-form-item label="批量原价（GHS）" class="mb-0">
              <el-input-number v-model="skuBatchForm.original_price" class="w-full" :min="0" :precision="2" :step="0.01" controls-position="right" />
            </el-form-item>
            <el-form-item label="批量库存" class="mb-0">
              <el-input-number v-model="skuBatchForm.sellable_stock" class="w-full" :min="0" :precision="0" :step="1" step-strictly controls-position="right" />
            </el-form-item>
            <el-button type="primary" plain @click="applySkuBatchValues">应用到全部 SKU</el-button>
          </div>
        </div>

        <el-table :data="form.skus" border>
          <el-table-column label="规格组合" min-width="180">
            <template #default="{ row }: { row: SKUForm }">
              <span class="font-semibold text-slate-700">{{ skuDisplayName(row.specs) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="SKU 编码" min-width="180">
            <template #default="{ row }: { row: SKUForm }">
              <el-input :model-value="skuCodeDisplay(row)" readonly placeholder="系统自动生成" />
            </template>
          </el-table-column>
          <el-table-column label="SKU 图片（可选）" width="130" align="center">
            <template #default="{ row }: { row: SKUForm }">
              <el-upload
                v-model:file-list="row.image_files"
                class="sku-image-upload"
                :action="uploadAction"
                :headers="uploadHeaders"
                list-type="picture-card"
                accept="image/jpeg,image/png,image/webp"
                name="file"
                :limit="1"
                :before-upload="beforeImageUpload"
                :on-success="skuImageSuccessHandler(row)"
                :on-remove="skuImageRemoveHandler(row)"
                :on-error="handleUploadError"
                :on-exceed="handleSKUImageExceed"
              >
                <el-icon><Plus /></el-icon>
              </el-upload>
            </template>
          </el-table-column>
          <el-table-column label="零售价（GHS）" min-width="160">
            <template #default="{ row }: { row: SKUForm }">
              <el-input-number v-model="row.retail_price" class="w-full" :min="0" :precision="2" :step="0.01" controls-position="right" />
            </template>
          </el-table-column>
          <el-table-column label="原价（GHS）" min-width="160">
            <template #default="{ row }: { row: SKUForm }">
              <el-input-number v-model="row.original_price" class="w-full" :min="0" :precision="2" :step="0.01" controls-position="right" />
            </template>
          </el-table-column>
          <el-table-column label="库存量" min-width="160">
            <template #default="{ row }: { row: SKUForm }">
              <el-input-number v-model="row.sellable_stock" class="w-full" :min="0" :precision="0" :step="1" step-strictly controls-position="right" />
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <div class="flex justify-end gap-3 rounded-2xl bg-white p-4 shadow-sm">
        <el-button @click="router.push(adminPath('/products'))">取消</el-button>
        <el-button type="primary" native-type="submit" :loading="submitting" :disabled="videoUploading">保存</el-button>
      </div>
    </el-form>
  </section>
</template>

<style scoped>
.sku-image-upload :deep(.el-upload--picture-card),
.sku-image-upload :deep(.el-upload-list--picture-card .el-upload-list__item) {
  width: 64px;
  height: 64px;
}

</style>
