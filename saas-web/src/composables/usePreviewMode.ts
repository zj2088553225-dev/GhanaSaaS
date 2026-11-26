import { readonly, ref } from 'vue'
export { storefrontAuthEnabled } from '../config/storefrontAuth'

const dialogOpen = ref(false)

export function usePreviewMode() {
  function showPreviewMode() {
    dialogOpen.value = true
  }

  function hidePreviewMode() {
    dialogOpen.value = false
  }

  return {
    dialogOpen: readonly(dialogOpen),
    showPreviewMode,
    hidePreviewMode,
  }
}
