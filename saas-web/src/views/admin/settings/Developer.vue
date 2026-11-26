<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDeveloperCredentials, regenerateDeveloperSecret, type DeveloperCredentials } from '../../../api/developer'

const loading = ref(false)
const regenerating = ref(false)
const showSecret = ref(false)
const credentials = ref<DeveloperCredentials>({
  app_key: '',
  app_secret: '',
})

const maskedSecret = computed(() => {
  if (!credentials.value.app_secret) {
    return '-'
  }
  if (showSecret.value) {
    return credentials.value.app_secret
  }
  return '*'.repeat(24)
})

onMounted(() => {
  void loadCredentials()
})

async function loadCredentials() {
  loading.value = true
  try {
    credentials.value = await getDeveloperCredentials()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '开发者凭证加载失败')
  } finally {
    loading.value = false
  }
}

async function regenerateSecret() {
  try {
    await ElMessageBox.confirm('重置后旧 AppSecret 将立即失效，确认继续吗？', '重置秘钥', {
      confirmButtonText: '确认重置',
      cancelButtonText: '取消',
      type: 'warning',
    })
    regenerating.value = true
    credentials.value = await regenerateDeveloperSecret()
    showSecret.value = true
    ElMessage.success('AppSecret 已重置')
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '重置失败')
    }
  } finally {
    regenerating.value = false
  }
}
</script>

<template>
  <section v-loading="loading" class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">开发者设置</h2>
        <p class="mt-1 text-sm text-slate-500">用于第三方 ERP 调用 OpenAPI 的商户凭证</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <el-button @click="loadCredentials">刷新</el-button>
        <el-button type="danger" plain :loading="regenerating" @click="regenerateSecret">重置秘钥</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <div class="space-y-5">
        <div>
          <p class="mb-2 text-sm font-bold text-slate-500">AppKey</p>
          <el-input :model-value="credentials.app_key || '-'" readonly />
        </div>

        <div>
          <div class="mb-2 flex items-center justify-between">
            <p class="text-sm font-bold text-slate-500">AppSecret</p>
            <el-button text type="primary" @click="showSecret = !showSecret">
              {{ showSecret ? '隐藏' : '点击显示' }}
            </el-button>
          </div>
          <el-input :model-value="maskedSecret" readonly />
        </div>

        <el-alert
          type="warning"
          show-icon
          :closable="false"
          title="OpenAPI v2 请求必须携带 X-App-Key、X-Timestamp、X-Nonce、X-Signature-Version: v2 和 X-Signature；签名使用 HMAC-SHA256 并覆盖 method、path、query 和 body。"
        />
      </div>
    </el-card>
  </section>
</template>
