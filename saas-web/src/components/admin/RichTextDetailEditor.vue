<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import request from '../../utils/request'
import { sanitizeProductDetailHtml } from '../../utils/sanitizeProductDetail'

interface UploadResponse {
  data?: {
    url?: string
  }
}

const props = defineProps<{
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const editor = ref<HTMLElement>()
const imageInput = ref<HTMLInputElement>()
const mode = ref<'visual' | 'source'>('visual')
const uploading = ref(false)
let savedSelection: Range | null = null

watch(
  () => props.modelValue,
  () => {
    if (mode.value === 'visual') {
      syncEditorFromModel()
    }
  },
)

onMounted(syncEditorFromModel)

function updateModel(value: string) {
  emit('update:modelValue', value)
}

function syncEditorFromModel() {
  if (editor.value && editor.value.innerHTML !== props.modelValue) {
    editor.value.innerHTML = props.modelValue
  }
}

function syncModelFromEditor() {
  if (editor.value) {
    updateModel(editor.value.innerHTML)
  }
}

function sanitizeEditor() {
  const source = mode.value === 'visual' ? editor.value?.innerHTML || '' : props.modelValue
  const sanitized = sanitizeProductDetailHtml(source)
  updateModel(sanitized)
  if (mode.value === 'visual' && editor.value) {
    editor.value.innerHTML = sanitized
  }
}

async function switchMode(nextMode: 'visual' | 'source') {
  if (nextMode === mode.value) {
    return
  }
  sanitizeEditor()
  mode.value = nextMode
  if (nextMode === 'visual') {
    await nextTick()
    syncEditorFromModel()
  }
}

function rememberSelection() {
  const selection = window.getSelection()
  if (!selection?.rangeCount || !editor.value) {
    return
  }
  const range = selection.getRangeAt(0)
  if (editor.value.contains(range.commonAncestorContainer)) {
    savedSelection = range.cloneRange()
  }
}

function restoreSelection() {
  if (!savedSelection || !editor.value?.contains(savedSelection.commonAncestorContainer)) {
    return
  }
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(savedSelection)
}

function runCommand(command: string, value?: string) {
  if (!editor.value || mode.value !== 'visual') {
    return
  }
  editor.value.focus()
  restoreSelection()
  document.execCommand(command, false, value)
  syncModelFromEditor()
  rememberSelection()
}

function clearFormatting() {
  runCommand('removeFormat')
  runCommand('unlink')
}

function insertHTML(html: string) {
  const sanitized = sanitizeProductDetailHtml(html)
  if (!sanitized) {
    return
  }
  if (mode.value === 'source' || !editor.value) {
    updateModel(`${props.modelValue}${sanitized}`)
    return
  }

  editor.value.focus()
  restoreSelection()
  if (!document.execCommand('insertHTML', false, sanitized)) {
    editor.value.insertAdjacentHTML('beforeend', sanitized)
  }
  syncModelFromEditor()
  rememberSelection()
}

function insertText(text: string) {
  if (!text || !editor.value) {
    return
  }
  editor.value.focus()
  restoreSelection()
  if (!document.execCommand('insertText', false, text)) {
    editor.value.appendChild(document.createTextNode(text))
  }
  syncModelFromEditor()
  rememberSelection()
}

function escapeHTML(value: string) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
}

function unorderedListItemText(line: string) {
  return line.match(/^\s*(?:[•·*-])\s+(.+)$/)?.[1]?.trim() || ''
}

function orderedListItemText(line: string) {
  return line.match(/^\s*\d+[.)、]\s*(.+)$/)?.[1]?.trim() || ''
}

function plainTextListHTML(text: string) {
  const normalized = text.replace(/\r\n?/g, '\n')
  const contentLines = normalized.split('\n').map((line) => line.trim()).filter(Boolean)

  if (contentLines.length && contentLines.every((line) => Boolean(unorderedListItemText(line)))) {
    return `<ul>${contentLines.map((line) => `<li>${escapeHTML(unorderedListItemText(line))}</li>`).join('')}</ul>`
  }
  if (contentLines.length && contentLines.every((line) => Boolean(orderedListItemText(line)))) {
    return `<ol>${contentLines.map((line) => `<li>${escapeHTML(orderedListItemText(line))}</li>`).join('')}</ol>`
  }

  return ''
}

function plainTextToInsertHTML(text: string) {
  const normalized = text.replace(/\r\n?/g, '\n')
  const listHTML = plainTextListHTML(normalized)
  if (listHTML) {
    return listHTML
  }

  return normalized
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter(Boolean)
    .map((block) => `<p>${escapeHTML(block).replace(/\n/g, '<br>')}</p>`)
    .join('')
}

function insertPlainText(text: string) {
  const html = plainTextToInsertHTML(text)
  if (html) {
    insertHTML(html)
  } else {
    insertText(text)
  }
}

function normalizePlainText(value: string) {
  return value
    .replace(/\u00a0/g, ' ')
    .replace(/[ \t\f\v]+/g, ' ')
    .replace(/\s+\n/g, '\n')
    .replace(/\n\s+/g, '\n')
    .trim()
}

function directElementText(element: Element) {
  const pieces = Array.from(element.childNodes)
    .filter((node) => {
      if (node.nodeType !== Node.ELEMENT_NODE) {
        return true
      }
      const tag = (node as Element).tagName.toLowerCase()
      return tag !== 'ul' && tag !== 'ol'
    })
    .map((node) => {
      if (node.nodeType === Node.ELEMENT_NODE && (node as Element).tagName.toLowerCase() === 'br') {
        return '\n'
      }
      return node.textContent || ''
    })
  return normalizePlainText(pieces.join(' '))
}

function directChildLists(element: Element) {
  return Array.from(element.children).filter((child) => {
    const tag = child.tagName.toLowerCase()
    return tag === 'ul' || tag === 'ol'
  })
}

function renderListPlainText(list: Element, depth = 0) {
  const ordered = list.tagName.toLowerCase() === 'ol'
  const indent = '  '.repeat(depth)
  const lines: string[] = []
  let itemNumber = 1

  Array.from(list.children)
    .filter((child) => child.tagName.toLowerCase() === 'li')
    .forEach((item) => {
      const text = directElementText(item)
      if (text) {
        lines.push(`${indent}${ordered ? `${itemNumber}.` : '•'} ${text}`)
      }
      directChildLists(item).forEach((childList) => {
        lines.push(...renderListPlainText(childList, depth + 1))
      })
      itemNumber += 1
    })

  return lines
}

function renderNodePlainText(node: Node, depth = 0): string[] {
  if (node.nodeType === Node.TEXT_NODE) {
    const text = normalizePlainText(node.textContent || '')
    return text ? [text] : []
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return []
  }

  const element = node as Element
  const tag = element.tagName.toLowerCase()
  if (tag === 'ul' || tag === 'ol') {
    return renderListPlainText(element, depth)
  }
  if (tag === 'li') {
    const text = directElementText(element)
    const lines = text ? [`${'  '.repeat(depth)}• ${text}`] : []
    directChildLists(element).forEach((childList) => {
      lines.push(...renderListPlainText(childList, depth + 1))
    })
    return lines
  }
  if (['p', 'div', 'blockquote', 'figure', 'figcaption', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6'].includes(tag)) {
    const lines: string[] = []
    const text = directElementText(element)
    if (text) {
      lines.push(text)
    }
    directChildLists(element).forEach((childList) => {
      lines.push(...renderListPlainText(childList, depth))
    })
    return lines
  }
  if (tag === 'br') {
    return ['']
  }

  return Array.from(element.childNodes).flatMap((child) => renderNodePlainText(child, depth))
}

function clipboardPlainTextFromHTML(html: string) {
  const container = document.createElement('div')
  container.innerHTML = html
  return Array.from(container.childNodes)
    .flatMap((node) => renderNodePlainText(node))
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

function isPersistableImageURL(url: string) {
  const value = url.trim()
  return value.startsWith('/uploads/') || /^https?:\/\//i.test(value)
}

function validateImage(file: File) {
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
    ElMessage.error('详情图片仅支持 JPG、PNG、WEBP')
    return false
  }
  if (file.size > 5 * 1024 * 1024) {
    ElMessage.error('详情图片大小不能超过 5MB')
    return false
  }
  return true
}

async function uploadImage(file: File) {
  if (!validateImage(file)) {
    return
  }
  const body = new FormData()
  body.append('file', file)
  const response = await request.post<UploadResponse>('/admin/uploads/image', body)
  const url = response.data.data?.url || ''
  if (!url || !isPersistableImageURL(url)) {
    throw new Error('详情图片上传失败')
  }
  insertHTML(`<img src="${url}" alt="">`)
}

async function uploadImages(files: File[]) {
  if (!files.length) {
    return
  }
  uploading.value = true
  try {
    for (const file of files) {
      await uploadImage(file)
    }
    ElMessage.success('详情图片已上传并插入')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '详情图片上传失败')
  } finally {
    uploading.value = false
  }
}

async function handlePaste(event: ClipboardEvent) {
  const clipboard = event.clipboardData
  if (!clipboard) {
    return
  }
  event.preventDefault()
  rememberSelection()

  const imageFiles = Array.from(clipboard.files).filter((file) => file.type.startsWith('image/'))
  const html = clipboard.getData('text/html')
  const text = clipboard.getData('text/plain')
  if (html) {
    const sourceHTML = imageFiles.length ? html.replace(/<img\b[^>]*>/gi, '') : html
    const sanitizedHTML = sanitizeProductDetailHtml(sourceHTML)
    const listHTML = text ? plainTextListHTML(text) : ''
    insertHTML(listHTML && !/<\/?(?:ul|ol|li)\b/i.test(sanitizedHTML) ? listHTML : sourceHTML)
  } else if (text) {
    insertPlainText(text)
  }
  await uploadImages(imageFiles)
}

function handleCopy(event: ClipboardEvent) {
  const clipboard = event.clipboardData
  const selection = window.getSelection()
  if (!clipboard || !selection?.rangeCount || !editor.value || mode.value !== 'visual') {
    return
  }

  const range = selection.getRangeAt(0)
  if (!editor.value.contains(range.commonAncestorContainer)) {
    return
  }

  const container = document.createElement('div')
  container.appendChild(range.cloneContents())
  const html = sanitizeProductDetailHtml(container.innerHTML)
  const plainText = clipboardPlainTextFromHTML(html) || selection.toString()
  if (!html && !plainText) {
    return
  }

  event.preventDefault()
  clipboard.setData('text/html', html)
  clipboard.setData('text/plain', plainText)
}

function chooseImages() {
  rememberSelection()
  imageInput.value?.click()
}

async function handleImageSelection(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  await uploadImages(files)
}

function handleSourceInput(event: Event) {
  updateModel((event.target as HTMLTextAreaElement).value)
}
</script>

<template>
  <div class="overflow-hidden rounded-xl border border-slate-200 bg-white">
    <div class="sticky top-0 z-10 flex flex-wrap items-center gap-2 border-b border-slate-200 bg-slate-50 px-3 py-2">
      <button type="button" class="editor-tool font-black" :disabled="mode !== 'visual'" title="加粗" @mousedown.prevent="runCommand('bold')">
        B
      </button>
      <button type="button" class="editor-tool" :disabled="mode !== 'visual'" title="段落" @mousedown.prevent="runCommand('formatBlock', 'p')">
        段落
      </button>
      <button type="button" class="editor-tool" :disabled="mode !== 'visual'" title="无序列表" @mousedown.prevent="runCommand('insertUnorderedList')">
        • List
      </button>
      <button type="button" class="editor-tool" :disabled="mode !== 'visual'" title="有序列表" @mousedown.prevent="runCommand('insertOrderedList')">
        1. List
      </button>
      <button type="button" class="editor-tool" :disabled="mode !== 'visual'" title="清除格式" @mousedown.prevent="clearFormatting">
        清除格式
      </button>
      <button type="button" class="editor-tool" :disabled="uploading" title="插入图片" @mousedown.prevent="rememberSelection" @click="chooseImages">
        {{ uploading ? '上传中…' : '插入图片' }}
      </button>
      <span class="mx-1 h-6 w-px bg-slate-300" />
      <button type="button" class="editor-tool" :class="{ active: mode === 'visual' }" @click="switchMode('visual')">可视化</button>
      <button type="button" class="editor-tool" :class="{ active: mode === 'source' }" @click="switchMode('source')">HTML</button>
      <input ref="imageInput" class="hidden" type="file" accept="image/jpeg,image/png,image/webp" multiple @change="handleImageSelection" />
    </div>

    <div
      v-show="mode === 'visual'"
      ref="editor"
      class="rich-text-editor h-[520px] overflow-y-auto p-5 text-sm leading-7 text-slate-800 outline-none"
      contenteditable="true"
      data-placeholder="可直接粘贴其他网页的图文详情；剪贴板中的图片会上传到本站后插入。"
      @input="syncModelFromEditor"
      @blur="sanitizeEditor"
      @paste="handlePaste"
      @copy="handleCopy"
      @keyup="rememberSelection"
      @mouseup="rememberSelection"
    />

    <textarea
      v-show="mode === 'source'"
      :value="modelValue"
      class="block h-[520px] w-full resize-none overflow-y-auto border-0 bg-slate-950 p-5 font-mono text-sm leading-6 text-slate-100 outline-none"
      spellcheck="false"
      placeholder="<p>商品详情</p><img src=&quot;/uploads/detail.webp&quot;>"
      @input="handleSourceInput"
      @blur="sanitizeEditor"
    />
  </div>

  <p class="mt-3 rounded-lg bg-amber-50 px-4 py-3 text-xs leading-5 text-amber-800">
    粘贴图片文件会自动上传到本站。外部 http/https 图片地址会保留，但长期使用建议下载后上传，避免来源站点失效或限制访问。
  </p>
</template>

<style scoped>
.editor-tool {
  min-height: 32px;
  border-radius: 7px;
  padding: 4px 10px;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.editor-tool:hover:not(:disabled),
.editor-tool.active {
  background: #fff;
  color: #ea580c;
  box-shadow: 0 1px 3px rgb(15 23 42 / 0.12);
}

.editor-tool:disabled {
  cursor: not-allowed;
  opacity: 0.4;
}

.rich-text-editor:empty::before {
  color: #94a3b8;
  content: attr(data-placeholder);
  pointer-events: none;
}

.rich-text-editor :deep(img) {
  display: block;
  height: auto;
  margin: 12px auto;
  max-width: 100%;
}

.rich-text-editor :deep(p) {
  margin: 8px 0;
}

.rich-text-editor :deep(ul) {
  list-style: disc;
  margin: 0.75rem 0;
  padding-left: 1.5rem;
}

.rich-text-editor :deep(ol) {
  list-style: decimal;
  margin: 0.75rem 0;
  padding-left: 1.5rem;
}

.rich-text-editor :deep(li) {
  margin: 0.25rem 0;
}

.rich-text-editor :deep(li > ul),
.rich-text-editor :deep(li > ol) {
  margin: 0.25rem 0;
}

.rich-text-editor :deep(table) {
  border-collapse: collapse;
  width: 100%;
}

.rich-text-editor :deep(td),
.rich-text-editor :deep(th) {
  border: 1px solid #e2e8f0;
  padding: 8px;
}
</style>
