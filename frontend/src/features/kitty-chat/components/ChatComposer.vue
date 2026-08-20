<script setup lang="ts">
import { ref } from 'vue'

import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  modelValue: string
  attachments: File[]
  canSend: boolean
  streaming: boolean
  usageText: string
  endpointHost: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  files: [files: FileList]
  remove: [index: number]
  send: []
  stop: []
}>()

const fileInput = ref<HTMLInputElement | null>(null)

function handleInput(event: Event): void {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault()
  if (props.canSend) emit('send')
}

function handleFiles(event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.files?.length) emit('files', input.files)
  input.value = ''
}
</script>

<template>
  <div class="composer-dock">
    <div class="composer-box">
      <ul v-if="attachments.length" class="composer-files">
        <li v-for="(file, index) in attachments" :key="`${file.name}-${file.lastModified}`">
          <Icon name="document" size="xs" />
          <span>{{ file.name }}</span>
          <button type="button" title="移除附件" :aria-label="`移除 ${file.name}`" @click="emit('remove', index)">
            <Icon name="x" size="xs" />
          </button>
        </li>
      </ul>
      <textarea
        :value="modelValue"
        rows="3"
        maxlength="100000"
        placeholder="输入聊天内容，也可以上传图片、代码或文本文件..."
        :disabled="streaming"
        @input="handleInput"
        @keydown="handleKeydown"
      ></textarea>
      <footer>
        <div class="composer-actions">
          <input
            ref="fileInput"
            class="sr-only"
            type="file"
            multiple
            accept="image/*,.txt,.md,.json,.csv,.ts,.tsx,.js,.jsx,.py,.go,.rs,.java,.html,.css,.xml,.yaml,.yml,.toml,.ini,.log"
            @change="handleFiles"
          >
          <button type="button" :disabled="streaming" @click="fileInput?.click()">
            <Icon name="upload" size="sm" />上传文件
          </button>
          <span v-if="usageText">{{ usageText }}</span>
        </div>
        <button v-if="streaming" class="send-button stop" type="button" @click="emit('stop')">
          <Icon name="x" size="sm" />停止
        </button>
        <button v-else class="send-button" type="button" :disabled="!canSend" @click="emit('send')">
          发送<Icon name="paperAirplane" size="sm" />
        </button>
      </footer>
    </div>
    <p>网页试聊由 {{ endpointHost }} 直连当前站点模型 · Key 仅用于本次浏览器调用，请求经本站加密代理</p>
  </div>
</template>

<style scoped>
.composer-dock {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 4;
  padding: 20px 18px 10px;
  background: var(--workbench-bg);
}

.composer-box {
  width: min(820px, 100%);
  margin: 0 auto;
  border: 1px solid var(--workbench-line-strong);
  border-radius: 8px;
  background: var(--workbench-surface);
  box-shadow: 0 16px 35px rgb(15 23 42 / 10%);
}

textarea {
  display: block;
  width: 100%;
  min-height: 76px;
  resize: none;
  border: 0;
  padding: 13px 14px 4px;
  color: var(--workbench-ink);
  background: transparent;
  font-size: 12px;
  line-height: 1.6;
  outline: 0;
}

.composer-box:focus-within {
  border-color: var(--workbench-accent);
  box-shadow: 0 0 0 3px var(--workbench-accent-ring), 0 16px 35px rgb(15 23 42 / 10%);
}

.composer-box footer {
  display: flex;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 8px 8px;
}

.composer-actions {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.composer-actions button,
.send-button {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border-radius: 6px;
  padding: 0 10px;
  font-size: 10px;
}

.composer-actions button {
  border: 1px solid var(--workbench-line);
  color: var(--workbench-muted);
}

.composer-actions button:hover:not(:disabled) {
  border-color: var(--workbench-accent);
  color: var(--workbench-accent);
}

.composer-actions span {
  overflow: hidden;
  color: var(--workbench-muted);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.send-button {
  min-width: 78px;
  color: #fff;
  background: var(--workbench-accent);
  font-weight: 700;
}

.send-button:hover:not(:disabled) {
  background: var(--workbench-accent-strong);
}

.send-button.stop {
  background: #dc2626;
}

.composer-files {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  padding: 9px 10px 0;
}

.composer-files li {
  display: flex;
  max-width: 200px;
  flex: 0 0 auto;
  align-items: center;
  gap: 5px;
  border: 1px solid var(--workbench-line);
  border-radius: 5px;
  padding: 5px 7px;
  color: var(--workbench-muted);
  font-size: 9px;
}

.composer-files span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.composer-dock > p {
  margin: 8px auto 0;
  color: var(--workbench-muted);
  font-size: 9px;
  text-align: center;
}

@media (max-width: 560px) {
  .composer-dock { padding-inline: 8px; }
  .composer-dock > p { display: none; }
  .composer-actions span { display: none; }
}
</style>
