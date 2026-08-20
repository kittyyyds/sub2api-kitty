<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef, watch } from 'vue'

import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  accept?: string
  multiple?: boolean
  maxFiles?: number
  disabled?: boolean
  required?: boolean
}>(), {
  accept: 'image/png,image/jpeg,image/webp',
  multiple: true,
  maxFiles: 5,
  disabled: false,
  required: false,
})

const files = defineModel<File[]>({ required: true })
const inputRef = shallowRef<HTMLInputElement | null>(null)
const previews = shallowRef<Array<{ file: File; url: string }>>([])
const error = shallowRef('')
const countLabel = computed(() => `${files.value.length}/${props.maxFiles}`)

function rebuildPreviews(nextFiles: File[]): void {
  previews.value.forEach(({ url }) => URL.revokeObjectURL(url))
  previews.value = nextFiles.map((file) => ({
    file,
    url: file.type.startsWith('image/') ? URL.createObjectURL(file) : '',
  }))
}

function acceptsFile(file: File): boolean {
  const accepted = props.accept.split(',').map((value) => value.trim().toLowerCase()).filter(Boolean)
  if (!accepted.length) return true
  const type = file.type.toLowerCase()
  const name = file.name.toLowerCase()
  return accepted.some((rule) => {
    if (rule.startsWith('.')) return name.endsWith(rule)
    if (rule.endsWith('/*')) return type.startsWith(rule.slice(0, -1))
    return type === rule
  })
}

watch(files, rebuildPreviews, { immediate: true })

function addFiles(incoming: FileList | File[]): void {
  if (props.disabled) return
  error.value = ''
  const next = [...files.value]
  for (const file of Array.from(incoming)) {
    if (!acceptsFile(file)) {
      error.value = `不支持文件 ${file.name} 的格式。`
      continue
    }
    if (file.size > 20 * 1024 * 1024) {
      error.value = '单个文件不能超过 20MB。'
      continue
    }
    if (!next.some((item) => item.name === file.name && item.size === file.size)) next.push(file)
    if (!props.multiple || next.length >= props.maxFiles) break
  }
  files.value = next.slice(0, props.maxFiles)
}

function onChange(event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.files) addFiles(input.files)
  input.value = ''
}

function removeFile(index: number): void {
  if (props.disabled) return
  files.value = files.value.filter((_, itemIndex) => itemIndex !== index)
}

onBeforeUnmount(() => previews.value.forEach(({ url }) => URL.revokeObjectURL(url)))
</script>

<template>
  <div class="reference-control">
    <input ref="inputRef" class="sr-only" type="file" :accept="accept" :multiple="multiple" @change="onChange">
    <button
      type="button"
      class="reference-drop"
      :disabled="disabled || files.length >= maxFiles"
      @click="inputRef?.click()"
      @dragover.prevent
      @drop.prevent="addFiles($event.dataTransfer?.files || [])"
    >
      <Icon name="upload" size="md" />
      <span>选择或拖入参考文件</span>
      <small>{{ required ? '必填' : '可选' }} · 单个不超过 20MB · {{ countLabel }}</small>
    </button>
    <div v-if="previews.length" class="reference-previews">
      <figure v-for="(preview, index) in previews" :key="`${preview.file.name}-${preview.file.size}`">
        <img v-if="preview.url" :src="preview.url" alt="">
        <span v-else><Icon name="play" size="md" /></span>
        <figcaption :title="preview.file.name">{{ preview.file.name }}</figcaption>
        <button type="button" :aria-label="`移除 ${preview.file.name}`" @click="removeFile(index)">
          <Icon name="x" size="xs" />
        </button>
      </figure>
    </div>
    <p v-if="error" class="field-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.reference-control { display: grid; gap: 9px; }
.reference-drop {
  display: flex;
  min-height: 92px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 14px;
  border: 1px dashed var(--customer-line-strong);
  border-radius: 6px;
  color: var(--customer-muted);
  background: var(--customer-surface-soft);
}
.reference-drop:hover:not(:disabled) { border-color: var(--customer-cyan); color: var(--customer-cyan); }
.reference-drop:disabled { cursor: not-allowed; opacity: 0.62; }
.reference-drop span { font-size: 12px; font-weight: 700; }
.reference-drop small { font-size: 10px; }
.reference-previews { display: grid; grid-template-columns: repeat(auto-fill, minmax(112px, 1fr)); gap: 8px; }
.reference-previews figure { position: relative; min-width: 0; margin: 0; overflow: hidden; border: 1px solid var(--customer-line); border-radius: 5px; background: var(--customer-surface); }
.reference-previews img,
.reference-previews figure > span { display: grid; width: 100%; height: 78px; place-items: center; background: var(--customer-surface-soft); object-fit: contain; }
.reference-previews figcaption { overflow: hidden; padding: 6px 8px; color: var(--customer-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.reference-previews figure button { position: absolute; top: 5px; right: 5px; display: grid; width: 24px; height: 24px; place-items: center; border-radius: 4px; color: white; background: rgb(18 28 32 / 78%); }
.field-error { margin: 0; color: #c2413b; font-size: 11px; }
</style>
