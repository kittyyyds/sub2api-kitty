<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

import type { AppKeyOption } from '../composables/useAppCenter'

defineProps<{
  plannerOptions: readonly AppKeyOption[]
  mediaOptions: readonly AppKeyOption[]
  loading: boolean
  disabled: boolean
}>()

const plannerKeyId = defineModel<string>('plannerKeyId', { required: true })
const mediaKeyId = defineModel<string>('mediaKeyId', { required: true })
const plannerEnabled = defineModel<boolean>('plannerEnabled', { required: true })
</script>

<template>
  <section class="key-strip" aria-label="运行密钥">
    <div class="key-title"><Icon name="key" size="sm" /><span>运行密钥</span></div>
    <label class="planner-toggle">
      <input v-model="plannerEnabled" type="checkbox" :disabled="loading || disabled">
      <span>AI 策划</span>
    </label>
    <label v-if="plannerEnabled">
      <span>策划</span>
      <select v-model="plannerKeyId" :disabled="loading || disabled || !plannerOptions.length">
        <option v-if="!plannerOptions.length" value="">暂无可用密钥</option>
        <option v-for="option in plannerOptions" :key="option.id" :value="option.id">{{ option.label }}</option>
      </select>
    </label>
    <label>
      <span>媒体</span>
      <select v-model="mediaKeyId" :disabled="loading || disabled || !mediaOptions.length">
        <option v-if="!mediaOptions.length" value="">暂无可用媒体密钥</option>
        <option v-for="option in mediaOptions" :key="option.id" :value="option.id">{{ option.label }}</option>
      </select>
    </label>
    <RouterLink to="/keys" title="管理 API 密钥"><Icon name="cog" size="sm" /></RouterLink>
  </section>
</template>

<style scoped>
.key-strip { display: grid; grid-template-columns: auto auto minmax(0, 1fr) minmax(0, 1fr) 34px; align-items: end; gap: 10px; min-width: 0; padding: 12px; border: 1px solid var(--customer-line); border-radius: 7px; background: var(--customer-surface-soft); }
.key-title { display: flex; min-height: 38px; align-items: center; gap: 6px; color: var(--customer-copper); font-size: 11px; font-weight: 750; }
label { display: grid; gap: 5px; min-width: 0; }
label > span { color: var(--customer-muted); font-size: 10px; font-weight: 700; }
.planner-toggle { display: flex; min-height: 38px; align-items: center; gap: 7px; padding: 0 9px; border: 1px solid var(--customer-line); border-radius: 5px; background: var(--customer-surface); }
.planner-toggle input { width: 15px; height: 15px; accent-color: var(--customer-cyan); }
.planner-toggle > span { color: var(--customer-ink); white-space: nowrap; }
select { width: 100%; min-width: 0; min-height: 38px; padding: 7px 9px; border: 1px solid var(--customer-line); border-radius: 5px; color: var(--customer-ink); background: var(--customer-surface); font-size: 11px; }
a { display: grid; width: 34px; height: 38px; place-items: center; border: 1px solid var(--customer-line); border-radius: 5px; color: var(--customer-muted); background: var(--customer-surface); }
a:hover { border-color: var(--customer-cyan); color: var(--customer-cyan); }
@media (max-width: 780px) { .key-strip { grid-template-columns: 1fr 34px; } .key-title { grid-column: 1 / -1; } label { grid-column: 1; } a { grid-column: 2; grid-row: -2; } }
</style>
