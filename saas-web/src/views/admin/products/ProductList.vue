<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  confirmProductImport,
  deleteAdminProduct,
  downloadProductImportTemplate,
  getAdminProducts,
  previewProductImport,
  updateAdminProductStatus,
  type AdminProduct,
  type ProductImportPreview,
  type ProductImportPreviewProduct,
  type ProductStatus,
} from '../../../api/adminProducts'
import { formatMoney } from '../../../utils/money'
import { adminPath } from '../../../utils/adminPath.ts'
import { productDeletionErrorMessage } from '../../../utils/productDeletion'
import { imageVariantURL } from '../../../utils/imageVariants'

const router = useRouter()
const loading = ref(false)
const importDialogVisible = ref(false)
const importPreviewLoading = ref(false)
const importConfirming = ref(false)
const importFile = ref<File | null>(null)
const importPreview = ref<ProductImportPreview | null>(null)
const fileInputRef = ref<HTMLInputElement>()
const products = ref<AdminProduct[]>([])
const total = ref(0)
const query = reactive<{
  keyword: string
  status: '' | ProductStatus
  page: number
  page_size: number
}>({
  keyword: '',
  status: '',
  page: 1,
  page_size: 20,
})

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '上架', value: 'on_shelf' },
  { label: '下架', value: 'off_shelf' },
]

const tableData = computed(() => products.value)
const hasImportErrors = computed(() => Boolean(importPreview.value?.invalid_rows || importPreview.value?.errors.length))

onMounted(() => {
  void loadProducts()
})

async function loadProducts() {
  loading.value = true
  try {
    const page = await getAdminProducts({
      keyword: query.keyword || undefined,
      status: query.status || undefined,
      page: query.page,
      page_size: query.page_size,
    })
    products.value = page.items
    total.value = page.total
    query.page = page.page
    query.page_size = page.page_size
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '商品列表加载失败')
  } finally {
    loading.value = false
  }
}

function formatPrice(cents: number) {
  return formatMoney(cents)
}

function displayPrice(product: AdminProduct) {
  if (!product.skus.length) {
    return '-'
  }

  return formatPrice(Math.min(...product.skus.map((sku) => sku.retail_price)))
}

function displayStock(product: AdminProduct) {
  return product.skus.reduce((totalStock, sku) => totalStock + sku.sellable_stock, 0)
}

function productThumbnail(product: AdminProduct) {
  return imageVariantURL(product.main_image?.trim() || firstGalleryImage(product.slider_images), 'thumb')
}

function firstGalleryImage(value: unknown): string {
  if (Array.isArray(value)) {
    return value.find((item): item is string => typeof item === 'string' && Boolean(item.trim()))?.trim() || ''
  }

  if (typeof value === 'string' && value.trim()) {
    try {
      const parsed = JSON.parse(value) as unknown
      return firstGalleryImage(parsed)
    } catch {
      return ''
    }
  }

  return ''
}

function statusLabel(status: ProductStatus) {
  return status === 'on_shelf' ? '上架' : '下架'
}

function statusTagType(status: ProductStatus) {
  return status === 'on_shelf' ? 'success' : 'info'
}

async function searchProducts() {
  query.page = 1
  await loadProducts()
}

async function changePage(page: number) {
  query.page = page
  await loadProducts()
}

async function changePageSize(pageSize: number) {
  query.page_size = pageSize
  query.page = 1
  await loadProducts()
}

async function toggleStatus(product: AdminProduct) {
  const nextStatus: ProductStatus = product.status === 'on_shelf' ? 'off_shelf' : 'on_shelf'
  try {
    await updateAdminProductStatus(product.id, nextStatus)
    ElMessage.success(nextStatus === 'on_shelf' ? '商品已上架' : '商品已下架')
    await loadProducts()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '状态更新失败')
  }
}

async function removeProduct(product: AdminProduct) {
  try {
    await ElMessageBox.confirm(
      `确认删除商品「${product.name}」吗？已有订单记录的商品不能删除，请使用下架。`,
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
      },
    )
    await deleteAdminProduct(product.id)
    ElMessage.success('商品已删除')
    await loadProducts()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(productDeletionErrorMessage(error))
    }
  }
}

async function downloadImportTemplate() {
  try {
    const blob = await downloadProductImportTemplate()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'product-import-template.xlsx'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '模板下载失败')
  }
}

function openImportDialog() {
  importDialogVisible.value = true
  resetImportState()
}

function resetImportState() {
  importFile.value = null
  importPreview.value = null
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
  }
}

function chooseImportFile() {
  fileInputRef.value?.click()
}

async function handleImportFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  importPreview.value = null
  if (!file) {
    importFile.value = null
    return
  }
  if (!file.name.toLowerCase().endsWith('.xlsx')) {
    ElMessage.error('仅支持 .xlsx 文件')
    target.value = ''
    importFile.value = null
    return
  }
  if (file.size > 10 * 1024 * 1024) {
    ElMessage.error('Excel 文件不能超过 10MB')
    target.value = ''
    importFile.value = null
    return
  }
  importFile.value = file
  await previewImportFile()
}

async function previewImportFile() {
  if (!importFile.value) {
    ElMessage.warning('请先选择 Excel 文件')
    return
  }
  importPreviewLoading.value = true
  try {
    importPreview.value = await previewProductImport(importFile.value)
    if (hasImportErrors.value) {
      ElMessage.warning('Excel 存在错误，请修正后重新上传')
    } else {
      ElMessage.success('Excel 解析通过')
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : 'Excel 解析失败')
  } finally {
    importPreviewLoading.value = false
  }
}

async function confirmImportFile() {
  if (!importFile.value || !importPreview.value || hasImportErrors.value) {
    return
  }
  importConfirming.value = true
  try {
    const result = await confirmProductImport(importFile.value)
    ElMessage.success(`导入成功：${result.imported_products} 个商品，${result.imported_skus} 个 SKU`)
    importDialogVisible.value = false
    resetImportState()
    await loadProducts()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '商品导入失败')
  } finally {
    importConfirming.value = false
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">商品列表</h2>
        <p class="mt-1 text-sm text-slate-500">管理商品上下架、库存与 SKU 信息</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <el-button @click="downloadImportTemplate">下载导入模板</el-button>
        <el-button type="success" @click="openImportDialog">Excel 批量上品</el-button>
        <el-button type="primary" @click="router.push(adminPath('/products/create'))">新增商品</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <div class="mb-4 flex flex-col gap-3 md:flex-row md:items-center">
        <el-input v-model="query.keyword" class="md:max-w-xs" clearable placeholder="搜索商品名称" @keyup.enter="searchProducts" />
        <el-select v-model="query.status" class="md:w-40" placeholder="状态" @change="searchProducts">
          <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <el-button type="primary" @click="searchProducts">搜索</el-button>
        <el-button @click="loadProducts">刷新</el-button>
      </div>

      <el-table v-loading="loading" :data="tableData" border>
        <el-table-column label="商品图" width="92" align="center">
          <template #default="{ row }: { row: AdminProduct }">
            <img
              v-if="productThumbnail(row)"
              :src="productThumbnail(row)"
              :alt="row.name"
              class="mx-auto h-12 w-12 rounded-lg border border-slate-100 bg-slate-50 object-cover"
              loading="lazy"
            />
            <div
              v-else
              class="mx-auto flex h-12 w-12 items-center justify-center rounded-lg border border-dashed border-slate-200 bg-slate-50 px-1 text-center text-[10px] font-bold leading-3 text-slate-400"
            >
              暂无图片
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="220" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }: { row: AdminProduct }">
            <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="价格" width="140">
          <template #default="{ row }: { row: AdminProduct }">{{ displayPrice(row) }}</template>
        </el-table-column>
        <el-table-column label="库存" width="120">
          <template #default="{ row }: { row: AdminProduct }">{{ displayStock(row) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }: { row: AdminProduct }">
            <div class="flex flex-wrap gap-2">
              <el-button size="small" @click="router.push(adminPath(`/products/${row.id}/edit`))">编辑</el-button>
              <el-button size="small" :type="row.status === 'on_shelf' ? 'warning' : 'success'" @click="toggleStatus(row)">
                {{ row.status === 'on_shelf' ? '下架' : '上架' }}
              </el-button>
              <el-button size="small" type="danger" @click="removeProduct(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="mt-4 flex justify-end">
        <el-pagination
          background
          layout="total, sizes, prev, pager, next"
          :current-page="query.page"
          :page-size="query.page_size"
          :page-sizes="[10, 20, 50]"
          :total="total"
          @current-change="changePage"
          @size-change="changePageSize"
        />
      </div>
    </el-card>

    <el-dialog v-model="importDialogVisible" title="Excel 批量上品" width="860px" @closed="resetImportState">
      <div class="space-y-4">
        <input ref="fileInputRef" class="hidden" type="file" accept=".xlsx" @change="handleImportFileChange" />

        <div class="rounded-lg border border-dashed border-slate-200 bg-slate-50 p-4">
          <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div>
              <p class="text-sm font-bold text-slate-900">{{ importFile?.name || '请选择 .xlsx 文件' }}</p>
              <p class="mt-1 text-xs text-slate-500">
                最大 10MB，最多 1000 行。平台类目路径必须填写到最末级，例如：食品饮料/饮料/碳酸饮料。SKU 编码由系统自动生成。
              </p>
            </div>
            <div class="flex flex-wrap gap-2">
              <el-button @click="chooseImportFile">选择文件</el-button>
              <el-button :disabled="!importFile" :loading="importPreviewLoading" @click="previewImportFile">重新解析</el-button>
            </div>
          </div>
        </div>

        <div v-if="importPreview" class="grid gap-3 md:grid-cols-3">
          <div class="rounded-lg bg-white p-4 text-center shadow-sm ring-1 ring-slate-100">
            <p class="text-xs font-bold text-slate-500">总行数</p>
            <p class="mt-1 text-2xl font-black text-slate-950">{{ importPreview.rows }}</p>
          </div>
          <div class="rounded-lg bg-white p-4 text-center shadow-sm ring-1 ring-slate-100">
            <p class="text-xs font-bold text-slate-500">有效行</p>
            <p class="mt-1 text-2xl font-black text-emerald-600">{{ importPreview.valid_rows }}</p>
          </div>
          <div class="rounded-lg bg-white p-4 text-center shadow-sm ring-1 ring-slate-100">
            <p class="text-xs font-bold text-slate-500">错误行</p>
            <p class="mt-1 text-2xl font-black" :class="hasImportErrors ? 'text-red-600' : 'text-slate-950'">{{ importPreview.invalid_rows }}</p>
          </div>
        </div>

        <el-alert
          v-if="importPreview && hasImportErrors"
          title="Excel 存在错误，修正后重新上传。确认导入已禁用。"
          type="error"
          :closable="false"
          show-icon
        />

        <el-table v-if="importPreview?.errors.length" :data="importPreview.errors" border max-height="220">
          <el-table-column prop="row" label="行号" width="90" />
          <el-table-column prop="field" label="字段" width="140" />
          <el-table-column prop="message" label="错误" min-width="260" />
        </el-table>

        <el-table v-if="importPreview?.preview_products.length" :data="importPreview.preview_products" border max-height="260">
          <el-table-column prop="name" label="商品名称" min-width="180" />
          <el-table-column prop="platform_category_path" label="平台类目" min-width="180" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }: { row: ProductImportPreviewProduct }">
              <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="sku_count" label="SKU 数" width="90" />
          <el-table-column label="价格" width="160">
            <template #default="{ row }: { row: ProductImportPreviewProduct }">
              {{ row.min_price === row.max_price ? formatPrice(row.min_price) : `${formatPrice(row.min_price)} - ${formatPrice(row.max_price)}` }}
            </template>
          </el-table-column>
          <el-table-column prop="total_stock" label="库存" width="100" />
        </el-table>
      </div>

      <template #footer>
        <el-button @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!importPreview || hasImportErrors" :loading="importConfirming" @click="confirmImportFile">确认导入</el-button>
      </template>
    </el-dialog>
  </section>
</template>
