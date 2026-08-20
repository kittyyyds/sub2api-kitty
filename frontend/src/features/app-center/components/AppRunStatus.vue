<script setup lang="ts">
import { computed } from 'vue'

import Icon from '@/components/icons/Icon.vue'

import { imageTaskFromRun } from '../imageStatus'
import type { ApplicationRun, ApplicationRunStep } from '../types'
import { videoTaskFromRun } from '../videoStatus'

const props = defineProps<{
  run: ApplicationRun | null
  steps: readonly ApplicationRunStep[]
  running: boolean
}>()

defineEmits<{ cancel: [] }>()

const displaySteps = computed(() => props.steps.length ? props.steps : props.run?.steps || [])
const runID = computed(() => props.run ? String(props.run.id) : '')
const shortRunID = computed(() => runID.value.length > 10 ? runID.value.slice(0, 8) : runID.value)
const canCancel = computed(() => props.running || ['pending', 'running'].includes(props.run?.status || ''))
const hasSubmittedMedia = computed(() => !!videoTaskFromRun(props.run)?.requestId
  || !!imageTaskFromRun(props.run)?.taskId)

function executorLabel(value?: string): string {
  const labels: Record<string, string> = {
    chat_completions: '策划', image_generation: '生图', image_edit: '图片精修',
    video_generation: '视频生成', video_edit: '视频编辑', video_extension: '视频续写',
  }
  return labels[value || ''] || value || '等待'
}
</script>

<template>
  <section v-if="run || running" class="run-status" aria-live="polite">
    <header>
      <div><span>运行编排</span><strong :title="runID">{{ run ? `#${shortRunID}` : '正在创建' }}</strong></div>
      <button v-if="canCancel" type="button" @click="$emit('cancel')"><Icon name="x" size="sm" />{{ hasSubmittedMedia ? '停止等待' : '取消运行' }}</button>
      <span v-else class="run-state" :data-status="run?.status">{{ run?.status || 'pending' }}</span>
    </header>
    <div class="step-track">
      <div v-if="!displaySteps.length" class="step-item running"><span class="step-dot" /><div><strong>建立运行</strong><small>正在读取编排步骤</small></div></div>
      <div v-for="(step, index) in displaySteps" :key="step.step_key" class="step-item" :class="step.status">
        <span class="step-dot"><Icon v-if="step.status === 'completed'" name="check" size="xs" /><Icon v-else-if="step.status === 'failed'" name="x" size="xs" /></span>
        <div><strong>{{ String(index + 1).padStart(2, '0') }} · {{ executorLabel(step.executor) }}</strong><small>{{ step.status === 'running' ? '执行中' : step.status === 'completed' ? '已完成' : step.status === 'failed' ? (step.error_message || '失败') : '等待执行' }}</small></div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.run-status { border: 1px solid var(--customer-line); border-radius: 7px; background: var(--customer-surface); }
header { display: flex; min-height: 54px; align-items: center; justify-content: space-between; gap: 12px; padding: 11px 14px; border-bottom: 1px solid var(--customer-line); }
header > div { display: flex; min-width: 0; align-items: baseline; gap: 8px; }
header span { color: var(--customer-muted); font-size: 10px; font-weight: 700; }
header strong { overflow: hidden; color: var(--customer-ink); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
header button { display: inline-flex; min-height: 32px; align-items: center; gap: 5px; padding: 6px 9px; border: 1px solid var(--customer-line); border-radius: 5px; color: #b6413a; background: var(--customer-surface); font-size: 11px; }
.run-state { padding: 4px 7px; border-radius: 4px; background: var(--customer-surface-soft); }
.step-track { display: grid; grid-template-columns: repeat(auto-fit, minmax(145px, 1fr)); gap: 1px; background: var(--customer-line); }
.step-item { display: grid; min-width: 0; grid-template-columns: 24px minmax(0, 1fr); align-items: center; gap: 8px; padding: 12px; background: var(--customer-surface); }
.step-dot { display: grid; width: 22px; height: 22px; place-items: center; border: 1px solid var(--customer-line-strong); border-radius: 50%; color: var(--customer-muted); }
.step-item.running .step-dot { border-color: var(--customer-cyan); background: var(--customer-cyan-soft); animation: status-pulse 1.2s ease-in-out infinite; }
.step-item.completed .step-dot { border-color: #3b8c75; color: #2f7764; background: rgb(59 140 117 / 12%); }
.step-item.failed .step-dot { border-color: #c75a52; color: #b6413a; background: rgb(199 90 82 / 12%); }
.step-item div { min-width: 0; }
.step-item strong,
.step-item small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.step-item strong { color: var(--customer-ink); font-size: 10px; }
.step-item small { margin-top: 3px; color: var(--customer-muted); font-size: 9px; }
@keyframes status-pulse { 50% { opacity: 0.55; } }
@media (prefers-reduced-motion: reduce) { .step-item.running .step-dot { animation: none; } }
</style>
