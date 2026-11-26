<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getStorefrontTrackingSetting,
  saveStorefrontTrackingSetting,
  type StorefrontTrackingSetting,
} from '../../../api/storefrontTracking'

const loading = ref(false)
const saving = ref(false)
const form = reactive<StorefrontTrackingSetting>({
  meta_pixel_enabled: false,
  meta_pixel_id: '',
  custom_head_enabled: false,
  custom_head_html: '',
})

onMounted(() => {
  void loadSetting()
})

async function loadSetting() {
  loading.value = true
  try {
    Object.assign(form, await getStorefrontTrackingSetting())
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '营销代码设置加载失败')
  } finally {
    loading.value = false
  }
}

async function saveSetting() {
  const pixelID = form.meta_pixel_id.trim()
  if (pixelID && !/^[0-9]{5,32}$/.test(pixelID)) {
    ElMessage.error('Meta Pixel ID 只允许填写 5～32 位数字；完整代码请放到高级 Head Code')
    return
  }
  if (form.meta_pixel_enabled && !pixelID) {
    ElMessage.error('启用 Meta Pixel 前请填写 Pixel ID')
    return
  }
  if (form.custom_head_html.length > 10000) {
    ElMessage.error('自定义 Head Code 不能超过 10000 个字符')
    return
  }

  saving.value = true
  try {
    const saved = await saveStorefrontTrackingSetting({
      ...form,
      meta_pixel_id: pixelID,
      custom_head_html: form.custom_head_html.trim(),
    })
    Object.assign(form, saved)
    ElMessage.success('前台营销代码设置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section v-loading="loading" class="space-y-4">
    <div class="rounded-2xl bg-white p-4 shadow-sm">
      <h2 class="text-xl font-black text-slate-950">前台营销代码</h2>
      <p class="mt-1 text-sm text-slate-500">配置 Meta Pixel 或高级 Head Code，仅注入商城前台，不影响 Admin 后台。</p>
    </div>

    <el-card shadow="never">
      <div class="space-y-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="font-black text-slate-900">Meta Pixel</p>
            <p class="mt-1 text-sm text-slate-500">推荐优先使用 Pixel ID，由系统生成标准脚本并追踪 SPA PageView。</p>
          </div>
          <el-switch v-model="form.meta_pixel_enabled" />
        </div>

        <div>
          <p class="mb-2 text-sm font-bold text-slate-600">Meta Pixel ID</p>
          <el-input
            v-model="form.meta_pixel_id"
            maxlength="32"
            placeholder="例如：123456789012345"
          />
          <p class="mt-2 text-xs text-slate-500">仅填写 5～32 位数字。完整 Pixel 代码请放到下方高级 Head Code。</p>
        </div>

        <el-divider />

        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="font-black text-slate-900">自定义 Head Code（高级）</p>
            <p class="mt-1 text-sm text-slate-500">仅 super_admin 可配置。请仅加入可信的验证、统计或营销代码。</p>
          </div>
          <el-switch v-model="form.custom_head_enabled" />
        </div>

        <el-input
          v-model="form.custom_head_html"
          type="textarea"
          :rows="12"
          maxlength="10000"
          show-word-limit
          placeholder="<meta ...> 或 <script>...</script>"
        />

        <el-alert
          type="warning"
          show-icon
          :closable="false"
          title="高风险权限：自定义代码可执行第三方脚本并读取前台页面数据。仅粘贴经过安全审查且来源可信的代码；系统过滤与 report-only CSP 不能替代代码审查。只有 super_admin 有权保存，tenant_admin 无权配置。"
        />

        <div class="flex justify-end">
          <el-button type="primary" :loading="saving" @click="saveSetting">保存设置</el-button>
        </div>
      </div>
    </el-card>
  </section>
</template>
