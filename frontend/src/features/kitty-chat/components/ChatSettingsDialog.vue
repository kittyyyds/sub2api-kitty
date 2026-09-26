<script setup lang="ts">
import { reactive, watch } from 'vue'

import Icon from '@/components/icons/Icon.vue'

import type { ChatSettings } from '../types'

const props = defineProps<{
  open: boolean
  settings: ChatSettings
}>()

const emit = defineEmits<{
  close: []
  save: [settings: ChatSettings]
}>()

const draft = reactive<ChatSettings>({ ...props.settings })

watch(() => props.open, (open) => {
  if (open) Object.assign(draft, props.settings)
})

function save(): void {
  emit('save', {
    ...draft,
    temperature: Math.max(0, Math.min(2, Number(draft.temperature) || 0)),
    topP: Math.max(0, Math.min(1, Number(draft.topP) || 0)),
    maxTokens: Math.max(1, Math.round(Number(draft.maxTokens) || 8192)),
  })
  emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="settings-backdrop" role="presentation" @mousedown.self="emit('close')">
      <section class="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="chat-settings-title">
        <header>
          <div>
            <span>CHAT RUNTIME</span>
            <h2 id="chat-settings-title">聊天设置</h2>
          </div>
          <button type="button" title="关闭" aria-label="关闭聊天设置" @click="emit('close')">
            <Icon name="x" size="sm" />
          </button>
        </header>

        <label class="wide-field">
          <span>系统提示词</span>
          <textarea v-model="draft.systemPrompt" rows="5" maxlength="20000" placeholder="可选，为当前会话设定角色和回答规则"></textarea>
        </label>

        <div class="settings-grid">
          <label>
            <span>温度</span>
            <input v-model.number="draft.temperature" type="number" min="0" max="2" step="0.1">
          </label>
          <label>
            <span>Top P</span>
            <input v-model.number="draft.topP" type="number" min="0" max="1" step="0.05">
          </label>
          <label>
            <span>最大输出 Token</span>
            <input v-model.number="draft.maxTokens" type="number" min="1" max="200000" step="1">
          </label>
          <label>
            <span>思考等级</span>
            <select v-model="draft.reasoningEffort">
              <option value="auto">自动</option>
              <option value="none">关闭</option>
              <option value="minimal">Minimal</option>
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
              <option value="xhigh">XHigh</option>
            </select>
          </label>
        </div>

        <footer>
          <button type="button" @click="emit('close')">取消</button>
          <button class="primary" type="button" @click="save">保存设置</button>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.settings-backdrop {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  padding: 16px;
  background: rgb(3 7 18 / 62%);
}

.settings-dialog {
  width: min(560px, 100%);
  border: 1px solid rgb(203 213 225);
  border-radius: 8px;
  padding: 18px;
  color: rgb(15 23 42);
  background: #fff;
  box-shadow: 0 24px 60px rgb(0 0 0 / 25%);
}

.settings-dialog header,
.settings-dialog footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.settings-dialog header span {
  color: #0f766e;
  font-size: 9px;
  font-weight: 800;
}

.settings-dialog h2 {
  margin: 3px 0 0;
  font-size: 17px;
  font-weight: 760;
}

.settings-dialog header button {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  border-radius: 5px;
  color: #64748b;
}

.settings-dialog header button:hover { background: #f1f5f9; }

.wide-field {
  display: block;
  margin-top: 18px;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}

label > span {
  display: block;
  margin-bottom: 6px;
  color: #64748b;
  font-size: 10px;
  font-weight: 650;
}

textarea,
input,
select {
  width: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  padding: 9px 10px;
  color: #0f172a;
  background: #fff;
  font-size: 11px;
  outline: 0;
}

textarea { resize: vertical; line-height: 1.6; }

textarea:focus,
input:focus,
select:focus {
  border-color: #0f766e;
  box-shadow: 0 0 0 3px rgb(15 118 110 / 14%);
}

.settings-dialog footer {
  justify-content: flex-end;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid #e2e8f0;
}

.settings-dialog footer button {
  min-height: 34px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  padding: 0 12px;
  color: #475569;
  font-size: 10px;
}

.settings-dialog footer .primary {
  border-color: #0f766e;
  color: #fff;
  background: #0f766e;
}

:global(.dark) .settings-dialog {
  border-color: #334155;
  color: #f8fafc;
  background: #111827;
}

:global(.dark) .settings-dialog header button:hover,
:global(.dark) textarea,
:global(.dark) input,
:global(.dark) select {
  border-color: #475569;
  color: #f8fafc;
  background: #172033;
}

:global(.dark) .settings-dialog footer { border-color: #334155; }
:global(.dark) .settings-dialog footer button { border-color: #475569; color: #cbd5e1; }
:global(.dark) .settings-dialog footer .primary { border-color: #2dd4bf; color: #042f2e; background: #2dd4bf; }

@media (max-width: 560px) {
  .settings-grid { grid-template-columns: 1fr; }
}
</style>
