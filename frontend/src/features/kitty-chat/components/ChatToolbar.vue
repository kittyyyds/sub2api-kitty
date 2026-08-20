<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import type { ApiKey } from '@/types'

const props = defineProps<{
  keys: ApiKey[]
  models: string[]
  selectedKeyId: string
  selectedModel: string
  loadingKeys: boolean
  loadingModels: boolean
  disabled: boolean
}>()

const emit = defineEmits<{
  'update:key': [value: string]
  'update:model': [value: string]
  refresh: []
  settings: []
  'export:markdown': []
  'export:json': []
}>()

function maskKey(value: string): string {
  if (value.length <= 12) return value
  return `${value.slice(0, 6)}...${value.slice(-4)}`
}

function keyLabel(key: ApiKey): string {
  return `${key.name || `Key ${key.id}`} · ${maskKey(key.key)} · ${key.group?.name || '未分组'}`
}

function inputValue(event: Event): string {
  return (event.target as HTMLSelectElement).value
}
</script>

<template>
  <header class="chat-toolbar">
    <div class="chat-selectors">
      <label>
        <span class="sr-only">API Key</span>
        <select
          :value="props.selectedKeyId"
          :disabled="disabled || loadingKeys || !keys.length"
          @change="emit('update:key', inputValue($event))"
        >
          <option v-if="loadingKeys" value="">正在加载 Key...</option>
          <option v-else-if="!keys.length" value="">请先创建 API Key</option>
          <option v-for="key in keys" :key="key.id" :value="String(key.id)">{{ keyLabel(key) }}</option>
        </select>
      </label>
      <label class="model-selector">
        <span class="sr-only">模型</span>
        <select
          :value="props.selectedModel"
          :disabled="disabled || loadingModels || !models.length"
          @change="emit('update:model', inputValue($event))"
        >
          <option v-if="loadingModels" value="">正在读取模型...</option>
          <option v-else-if="!models.length" value="">暂无可用模型</option>
          <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
        </select>
      </label>
    </div>

    <nav class="chat-tools" aria-label="聊天工具">
      <RouterLink to="/model-plaza?embedded=1">模型价格</RouterLink>
      <RouterLink to="/apps">图片工作室</RouterLink>
      <button type="button" title="导出 Markdown" aria-label="导出 Markdown" @click="emit('export:markdown')">
        <Icon name="download" size="sm" />
        <span>MD</span>
      </button>
      <button type="button" title="导出 JSON" aria-label="导出 JSON" @click="emit('export:json')">
        <Icon name="document" size="sm" />
        <span>JSON</span>
      </button>
      <button type="button" title="聊天设置" aria-label="聊天设置" @click="emit('settings')">
        <Icon name="cog" size="sm" />
      </button>
      <button type="button" title="刷新 Key 与模型" aria-label="刷新 Key 与模型" :disabled="loadingKeys" @click="emit('refresh')">
        <Icon name="refresh" size="sm" :class="{ spinning: loadingKeys || loadingModels }" />
      </button>
    </nav>
  </header>
</template>

<style scoped>
.chat-toolbar {
  display: flex;
  min-width: 0;
  min-height: 54px;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--workbench-line);
  background: color-mix(in srgb, var(--workbench-surface) 90%, transparent);
}

.chat-selectors,
.chat-tools {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
}

.chat-selectors {
  flex: 1 1 420px;
}

.chat-tools {
  max-width: 100%;
  flex: 0 0 auto;
  margin-left: auto;
  overflow-x: auto;
}

.chat-selectors label:first-child {
  width: auto;
  min-width: 160px;
  flex: 1.1 1 0;
}

.model-selector {
  width: auto;
  min-width: 140px;
  flex: 1 1 0;
}

select {
  width: 100%;
  height: 36px;
  min-width: 0;
  border: 1px solid var(--workbench-line);
  border-radius: 6px;
  padding: 0 30px 0 10px;
  color: var(--workbench-ink);
  background: var(--workbench-surface);
  font-size: 10px;
  outline: 0;
}

select:focus-visible {
  border-color: var(--workbench-accent);
  box-shadow: 0 0 0 3px var(--workbench-accent-ring);
}

.chat-tools a,
.chat-tools button {
  display: inline-flex;
  height: 34px;
  align-items: center;
  justify-content: center;
  gap: 4px;
  border: 1px solid var(--workbench-line);
  border-radius: 6px;
  padding: 0 9px;
  color: var(--workbench-muted);
  background: var(--workbench-surface);
  font-size: 9px;
  white-space: nowrap;
}

.chat-tools a:hover,
.chat-tools button:hover:not(:disabled) {
  border-color: var(--workbench-accent);
  color: var(--workbench-accent);
}

.spinning {
  animation: spin .8s linear infinite;
}

@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 760px) {
  .chat-toolbar {
    align-items: stretch;
    flex-direction: column;
    flex-wrap: nowrap;
    justify-content: flex-start;
  }

  .chat-selectors {
    width: 100%;
    flex: 0 0 auto;
  }

  .chat-selectors label:first-child,
  .model-selector {
    width: auto;
    min-width: 0;
    flex: 1 1 0;
  }

  .chat-tools {
    width: 100%;
    margin-left: 0;
    gap: 2px;
    overflow-x: auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .spinning { animation: none; }
}
</style>
