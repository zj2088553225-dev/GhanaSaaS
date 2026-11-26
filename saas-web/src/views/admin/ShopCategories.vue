<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus } from '@element-plus/icons-vue'
import { createCategory, deleteCategory, getCategories, updateCategory, type Category, type CategoryPayload } from '../../api/categories'

interface CategoryFormModel {
  id?: number
  parent_id: number | null
  name: string
  sort: number
  icon: string
  is_active: boolean
}

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const formRef = ref<FormInstance>()
const categories = ref<Category[]>([])
const form = reactive<CategoryFormModel>(emptyForm())

const rules: FormRules<CategoryFormModel> = {
  name: [{ required: true, message: '请输入分类名称', trigger: 'blur' }],
}

const categoryTree = computed(() => buildCategoryTree(categories.value))
const parentOptions = computed(() => categories.value.filter((item) => item.id !== form.id))

onMounted(() => {
  void loadCategories()
})

function emptyForm(): CategoryFormModel {
  return {
    parent_id: null,
    name: '',
    sort: 0,
    icon: '',
    is_active: true,
  }
}

async function loadCategories() {
  loading.value = true
  try {
    categories.value = await getCategories()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '店铺分类加载失败')
  } finally {
    loading.value = false
  }
}

function buildCategoryTree(items: Category[]) {
  const childrenByParent = new Map<number, Category[]>()
  const roots: Category[] = []
  for (const item of items) {
    if (!item.parent_id) {
      roots.push(item)
      continue
    }
    childrenByParent.set(item.parent_id, [...(childrenByParent.get(item.parent_id) || []), item])
  }
  const attach = (item: Category): Category & { children?: Category[] } => ({
    ...item,
    children: (childrenByParent.get(item.id) || []).map(attach),
  })
  return roots.map(attach)
}

function openCreate(parent?: Category) {
  Object.assign(form, emptyForm(), { parent_id: parent?.id ?? null })
  dialogVisible.value = true
}

function openEdit(category: Category) {
  Object.assign(form, {
    id: category.id,
    parent_id: category.parent_id,
    name: category.name,
    sort: category.sort,
    icon: category.icon,
    is_active: category.is_active,
  })
  dialogVisible.value = true
}

function buildPayload(): CategoryPayload {
  return {
    parent_id: form.parent_id,
    name: form.name.trim(),
    sort: form.sort || 0,
    icon: form.icon.trim(),
    is_active: form.is_active,
  }
}

async function saveCategory() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) {
    return
  }
  saving.value = true
  try {
    const payload = buildPayload()
    if (form.id) {
      await updateCategory(form.id, payload)
      ElMessage.success('店铺分类已更新')
    } else {
      await createCategory(payload)
      ElMessage.success('店铺分类已创建')
    }
    dialogVisible.value = false
    await loadCategories()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function removeCategory(category: Category) {
  try {
    await ElMessageBox.confirm(`确认删除店铺分类「${category.name}」吗？`, '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await deleteCategory(category.id)
    ElMessage.success('店铺分类已删除')
    await loadCategories()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '删除失败')
    }
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">店铺分类</h2>
        <p class="mt-1 text-sm text-slate-500">维护当前店铺内展示分类和商家自定义分组</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate()">新增分类</el-button>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="categoryTree" row-key="id" border :tree-props="{ children: 'children' }">
        <el-table-column prop="name" label="分类名称" min-width="180" />
        <el-table-column prop="sort" label="排序" width="100" />
        <el-table-column prop="icon" label="图标" min-width="160" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }: { row: Category }">
            <el-tag :type="row.is_active ? 'success' : 'info'">{{ row.is_active ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="250" fixed="right">
          <template #default="{ row }: { row: Category }">
            <div class="flex flex-wrap gap-2">
              <el-button size="small" :icon="Plus" @click="openCreate(row)">子分类</el-button>
              <el-button size="small" :icon="Edit" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="danger" :icon="Delete" @click="removeCategory(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑店铺分类' : '新增店铺分类'" width="520px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="上级分类">
          <el-select v-model="form.parent_id" class="w-full" clearable filterable placeholder="根分类">
            <el-option :value="null" label="根分类" />
            <el-option v-for="item in parentOptions" :key="item.id" :value="item.id" :label="item.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="分类名称" prop="name">
          <el-input v-model="form.name" maxlength="120" show-word-limit />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" :step="1" />
        </el-form-item>
        <el-form-item label="图标">
          <el-input v-model="form.icon" placeholder="图标 URL 或标识" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.is_active" active-text="启用" inactive-text="停用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveCategory">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>
