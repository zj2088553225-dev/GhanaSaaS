<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules, UploadFile, UploadProps, UploadRawFile, UploadUserFile } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { getShopProfile, updateShopProfile, type ShopProfile, type ShopProfilePayload } from '../../api/adminShopProfile'
import { useAuthStore } from '../../stores/auth'

interface UploadResponse {
  data?: {
    url?: string
  }
  error?: string
}

const authStore = useAuthStore()
const { t } = useI18n()
const formRef = ref<FormInstance>()
const loading = ref(false)
const saving = ref(false)
const logoFiles = ref<UploadUserFile[]>([])
const bannerFiles = ref<UploadUserFile[]>([])
const uploadAction = '/api/v1/admin/upload'
const uploadHeaders = computed(() => (authStore.token ? { Authorization: `Bearer ${authStore.token}` } : {}))

const form = reactive<ShopProfilePayload>({
  slug: '',
  shop_name: '',
  logo_url: '',
  banner_url: '',
  description: '',
  phone: '',
  whatsapp: '',
  address: '',
  business_hours: '',
  is_public: false,
  translations: {
    zh: {
      shop_name: '',
      description: '',
      address: '',
      business_hours: '',
    },
  },
})

const translationForm = reactive({
  shop_name: '',
  description: '',
  address: '',
  business_hours: '',
})

const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
const rules: FormRules<ShopProfilePayload> = {
  shop_name: [
    { required: true, message: '请输入店铺名称', trigger: 'blur' },
    { max: 120, message: '店铺名称不能超过 120 个字符', trigger: 'blur' },
  ],
  slug: [
    { required: true, message: '请输入公开主页 slug', trigger: 'blur' },
    { pattern: slugPattern, message: '只支持小写字母、数字和短横线，且不能以短横线开头或结尾', trigger: 'blur' },
    { max: 80, message: 'slug 不能超过 80 个字符', trigger: 'blur' },
  ],
  logo_url: [{ max: 255, message: 'Logo URL 不能超过 255 个字符', trigger: 'blur' }],
  banner_url: [{ max: 255, message: 'Banner URL 不能超过 255 个字符', trigger: 'blur' }],
  description: [{ max: 2000, message: '简介不能超过 2000 个字符', trigger: 'blur' }],
  phone: [{ max: 64, message: '电话不能超过 64 个字符', trigger: 'blur' }],
  whatsapp: [{ max: 64, message: 'WhatsApp 不能超过 64 个字符', trigger: 'blur' }],
  address: [{ max: 255, message: '地址不能超过 255 个字符', trigger: 'blur' }],
  business_hours: [{ max: 255, message: '营业时间不能超过 255 个字符', trigger: 'blur' }],
}

onMounted(() => {
  void loadProfile()
})

async function loadProfile() {
  loading.value = true
  try {
    const profile = await getShopProfile()
    applyProfile(profile)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '店铺资料加载失败')
  } finally {
    loading.value = false
  }
}

function applyProfile(profile: ShopProfile) {
  Object.assign(form, {
    slug: profile.slug || '',
    shop_name: profile.shop_name || '',
    logo_url: profile.logo_url || '',
    banner_url: profile.banner_url || '',
    description: profile.description || '',
    phone: profile.phone || '',
    whatsapp: profile.whatsapp || '',
    address: profile.address || '',
    business_hours: profile.business_hours || '',
    is_public: Boolean(profile.is_public),
  })
  Object.assign(translationForm, {
    shop_name: profile.translations?.zh?.shop_name || '',
    description: profile.translations?.zh?.description || '',
    address: profile.translations?.zh?.address || '',
    business_hours: profile.translations?.zh?.business_hours || '',
  })
  logoFiles.value = form.logo_url ? [urlToUploadFile(form.logo_url)] : []
  bannerFiles.value = form.banner_url ? [urlToUploadFile(form.banner_url)] : []
}

function normalizeSlug() {
  form.slug = form.slug.trim().toLowerCase()
  void formRef.value?.validateField('slug')
}

function urlToUploadFile(url: string): UploadUserFile {
  return {
    name: url.split('/').pop() || url,
    url,
  }
}

function extractUploadURL(response: unknown) {
  const uploadResponse = response as UploadResponse
  return uploadResponse.data?.url || ''
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

const handleLogoSuccess: UploadProps['onSuccess'] = (response: unknown, uploadFile: UploadFile) => {
  const url = extractUploadURL(response)
  if (!url) {
    ElMessage.error('Logo 上传失败')
    return
  }
  form.logo_url = url
  logoFiles.value = [{ name: uploadFile.name, url }]
  void formRef.value?.validateField('logo_url')
}

const handleBannerSuccess: UploadProps['onSuccess'] = (response: unknown, uploadFile: UploadFile) => {
  const url = extractUploadURL(response)
  if (!url) {
    ElMessage.error('Banner 上传失败')
    return
  }
  form.banner_url = url
  bannerFiles.value = [{ name: uploadFile.name, url }]
  void formRef.value?.validateField('banner_url')
}

const handleLogoRemove: UploadProps['onRemove'] = () => {
  form.logo_url = ''
  logoFiles.value = []
}

const handleBannerRemove: UploadProps['onRemove'] = () => {
  form.banner_url = ''
  bannerFiles.value = []
}

const handleUploadError: UploadProps['onError'] = (error) => {
  ElMessage.error(error.message || '图片上传失败')
}

const handleLogoExceed: UploadProps['onExceed'] = () => {
  ElMessage.warning('Logo 只能上传 1 张，请先移除当前图片')
}

const handleBannerExceed: UploadProps['onExceed'] = () => {
  ElMessage.warning('Banner 只能上传 1 张，请先移除当前图片')
}

async function saveProfile() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) {
    return
  }

  saving.value = true
  try {
    const saved = await updateShopProfile({
      slug: form.slug.trim(),
      shop_name: form.shop_name.trim(),
      logo_url: form.logo_url.trim(),
      banner_url: form.banner_url.trim(),
      description: form.description.trim(),
      phone: form.phone.trim(),
      whatsapp: form.whatsapp.trim(),
      address: form.address.trim(),
      business_hours: form.business_hours.trim(),
      is_public: form.is_public,
      translations: {
        zh: {
          shop_name: translationForm.shop_name.trim(),
          description: translationForm.description.trim(),
          address: translationForm.address.trim(),
          business_hours: translationForm.business_hours.trim(),
        },
      },
    })
    applyProfile(saved)
    ElMessage.success('店铺资料已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section v-loading="loading" class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">店铺公开主页</h2>
        <p class="mt-1 text-sm text-slate-500">维护商家单页主页资料与公开状态</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <el-button @click="loadProfile">刷新</el-button>
        <el-button type="primary" :loading="saving" @click="saveProfile">保存</el-button>
      </div>
    </div>

    <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" @submit.prevent="saveProfile">
      <el-card shadow="never">
        <template #header>基础资料</template>

        <el-tabs class="admin-translation-tabs">
          <el-tab-pane :label="t('adminTranslations.english')">
            <el-form-item label="店铺名称" prop="shop_name">
              <el-input v-model="form.shop_name" maxlength="120" show-word-limit placeholder="请输入店铺名称" />
            </el-form-item>

            <el-form-item label="店铺简介" prop="description">
              <el-input v-model="form.description" type="textarea" :rows="5" maxlength="2000" show-word-limit placeholder="请输入店铺简介" />
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane :label="t('adminTranslations.chinese')">
            <el-form-item label="中文店铺名称">
              <el-input v-model="translationForm.shop_name" maxlength="120" show-word-limit placeholder="可选；不填则前台 fallback" />
            </el-form-item>

            <el-form-item label="中文店铺简介">
              <el-input v-model="translationForm.description" type="textarea" :rows="5" maxlength="2000" show-word-limit placeholder="可选；不填则前台 fallback" />
            </el-form-item>
          </el-tab-pane>
        </el-tabs>

        <el-form-item label="公开 Slug" prop="slug">
          <el-input v-model="form.slug" maxlength="80" placeholder="ghana-market" @blur="normalizeSlug">
            <template #prepend>/shops/</template>
          </el-input>
          <p class="mt-1 text-xs text-slate-500">只支持小写字母、数字和短横线，例如 ghana-market。</p>
        </el-form-item>

        <el-form-item label="公开状态" prop="is_public">
          <el-switch v-model="form.is_public" active-text="公开" inactive-text="隐藏" />
        </el-form-item>
      </el-card>

      <el-card class="mt-4" shadow="never">
        <template #header>图片</template>

        <div class="grid gap-5 lg:grid-cols-2">
          <div>
            <p class="mb-2 text-sm font-bold text-slate-600">Logo</p>
            <el-upload
              v-model:file-list="logoFiles"
              :action="uploadAction"
              :headers="uploadHeaders"
              list-type="picture-card"
              accept="image/jpeg,image/png,image/webp"
              name="file"
              :limit="1"
              :before-upload="beforeImageUpload"
              :on-success="handleLogoSuccess"
              :on-remove="handleLogoRemove"
              :on-error="handleUploadError"
              :on-exceed="handleLogoExceed"
            >
              <el-icon><Plus /></el-icon>
            </el-upload>
            <el-form-item class="mt-3" label-width="0" prop="logo_url">
              <el-input v-model="form.logo_url" clearable placeholder="或直接输入 Logo URL" />
            </el-form-item>
          </div>

          <div>
            <p class="mb-2 text-sm font-bold text-slate-600">Banner</p>
            <el-upload
              v-model:file-list="bannerFiles"
              :action="uploadAction"
              :headers="uploadHeaders"
              list-type="picture-card"
              accept="image/jpeg,image/png,image/webp"
              name="file"
              :limit="1"
              :before-upload="beforeImageUpload"
              :on-success="handleBannerSuccess"
              :on-remove="handleBannerRemove"
              :on-error="handleUploadError"
              :on-exceed="handleBannerExceed"
            >
              <el-icon><Plus /></el-icon>
            </el-upload>
            <el-form-item class="mt-3" label-width="0" prop="banner_url">
              <el-input v-model="form.banner_url" clearable placeholder="或直接输入 Banner URL" />
            </el-form-item>
          </div>
        </div>
      </el-card>

      <el-card class="mt-4" shadow="never">
        <template #header>联系方式</template>

        <div class="grid gap-4 lg:grid-cols-2">
          <el-form-item label="电话" prop="phone">
            <el-input v-model="form.phone" maxlength="64" placeholder="联系电话" />
          </el-form-item>

          <el-form-item label="WhatsApp" prop="whatsapp">
            <el-input v-model="form.whatsapp" maxlength="64" placeholder="WhatsApp 号码或链接" />
          </el-form-item>
        </div>

        <el-tabs class="admin-translation-tabs">
          <el-tab-pane :label="t('adminTranslations.english')">
            <div class="grid gap-4 lg:grid-cols-2">
              <el-form-item label="地址" prop="address">
                <el-input v-model="form.address" maxlength="255" placeholder="店铺地址" />
              </el-form-item>

              <el-form-item label="营业时间" prop="business_hours">
                <el-input v-model="form.business_hours" maxlength="255" placeholder="例如 Mon-Sat 09:00-18:00" />
              </el-form-item>
            </div>
          </el-tab-pane>
          <el-tab-pane :label="t('adminTranslations.chinese')">
            <div class="grid gap-4 lg:grid-cols-2">
              <el-form-item label="中文地址">
                <el-input v-model="translationForm.address" maxlength="255" placeholder="可选；不填则前台 fallback" />
              </el-form-item>

              <el-form-item label="中文营业时间">
                <el-input v-model="translationForm.business_hours" maxlength="255" placeholder="可选；不填则前台 fallback" />
              </el-form-item>
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-card>
    </el-form>
  </section>
</template>
