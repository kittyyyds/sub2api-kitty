<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@vueuse/core'
import type { IQResult, IQSummary } from '@/api/admin/iqInterval'

const props = defineProps<{ summary?: IQSummary; error?: boolean }>()
const now = useNow({ interval: 1000 })
const results = computed(() => props.summary?.results.slice(0, 20) ?? [])
const latest = computed(() => results.value[0])
const cells = computed(() => [...Array.from({ length: 20 - results.value.length }, () => null), ...results.value.slice().reverse()])
const correct = computed(() => results.value.filter(r => r.verdict === 'correct').length)
const running = computed(() => !!props.summary?.running_until && Date.parse(props.summary.running_until) > now.value.getTime())
const state = computed(() => {
  if (!props.summary) return '未设置'
  if (running.value) return '测试中'
  if (!props.summary.enabled) return '已暂停'
  const remaining = props.summary.next_run_at ? Date.parse(props.summary.next_run_at) - now.value.getTime() : 0
  return remaining > 0 ? `完成后等待 ${Math.ceil(remaining / 1000)} 秒` : '等待 / 排队'
})
function resultLabel(result: IQResult): string {
  if (result.verdict === 'error') return '未完成'
  if (result.verdict === 'correct') return '匹配21'
  return result.answer.trim() ? `未匹配 · 答案：${result.answer.trim()}` : '未匹配 · 未返回答案'
}
const headline = computed(() => props.error ? '状态读取失败' : running.value ? '测试中' : latest.value ? resultLabel(latest.value) : '未测试')
const color = computed(() => props.error ? 'text-red-600 dark:text-red-400' : running.value ? 'text-blue-600 dark:text-blue-400' : latest.value?.verdict === 'correct' ? 'text-emerald-600 dark:text-emerald-400' : latest.value?.verdict === 'error' ? 'text-red-600 dark:text-red-400' : latest.value ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500')
function tooltip(result: IQResult | null): string {
  if (!result) return '未测试'
  const detail = result.verdict === 'error' ? `报错：${result.error || '请求失败，请查看测试详情'}` : `答案：${result.answer.trim() || '未返回答案'}`
  return `${detail}\n模型：${result.model} / ${result.reasoning}\n时间：${new Date(result.finished_at).toLocaleString()}`
}
</script>

<template>
  <div class="mt-1.5 w-[260px] min-w-0 max-w-full whitespace-normal" data-testid="iq-history-strip">
    <p class="line-clamp-2 [overflow-wrap:anywhere] text-xs font-medium" :class="color" :title="`答案测试：${headline}`" data-testid="iq-headline" role="status">答案测试：{{ headline }}</p>
    <p v-if="!error && latest?.verdict === 'error'" class="mt-0.5 line-clamp-2 [overflow-wrap:anywhere] text-[11px] text-red-600 dark:text-red-400" :title="tooltip(latest)" data-testid="iq-error">{{ running ? '上次报错：' : '' }}{{ latest.error || '请求失败，请查看测试详情' }}</p>
    <p v-else-if="!error && running && latest" class="mt-0.5 line-clamp-2 [overflow-wrap:anywhere] text-[10px] text-gray-500" :title="tooltip(latest)">上次：{{ resultLabel(latest) }}</p>
    <div class="mt-1 flex gap-0.5" aria-label="最近20次定时测试，由左到右从旧到新">
      <span v-for="(r, i) in cells" :key="r?.id ?? `empty-${i}`" class="h-3 min-w-0 flex-1 rounded-sm focus:outline focus:outline-2 focus:outline-blue-500" :class="!r ? 'bg-gray-300 dark:bg-dark-500' : r.verdict === 'correct' ? 'bg-emerald-500' : r.verdict === 'error' ? 'bg-red-500' : 'bg-amber-400'" :title="tooltip(r)" :aria-label="tooltip(r)" :tabindex="r ? 0 : undefined" :data-verdict="r?.verdict ?? 'untested'"></span>
    </div>
    <p v-if="error" class="mt-1 text-[10px] text-red-600">记录读取失败，已有记录可能过期</p>
    <p v-else class="mt-1 text-[10px] text-gray-500">{{ state }} · {{ correct }}/{{ results.length }} 匹配 · {{ results.length ? Math.round(correct / results.length * 100) : 0 }}% <span v-if="summary">· 间隔 {{ summary.interval_seconds }}s</span></p>
    <p v-if="summary" class="truncate text-[10px] text-gray-500" :title="latest?.model || summary.model">{{ latest?.model || summary.model }} / {{ latest?.reasoning || summary.reasoning }}<span v-if="latest"> · {{ new Date(latest.finished_at).toLocaleString() }}</span></p>
  </div>
</template>
