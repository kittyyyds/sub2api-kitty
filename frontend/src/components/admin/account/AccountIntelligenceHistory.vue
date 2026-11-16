<template>
  <div
    v-if="latest"
    class="mt-1 flex w-full max-w-[15rem] flex-col items-start gap-0.5 text-xs leading-4"
    data-testid="account-intelligence-summary"
    :aria-label="t(key + 'title')"
  >
    <button
      type="button"
      class="max-w-full truncate text-left font-medium focus-visible:rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
      :class="resultColor"
      :title="tooltip(latest)"
      data-testid="intelligence-result"
      @click="$emit('open')"
    >{{ resultLabel }}</button>
    <p
      class="max-w-full truncate tabular-nums text-gray-500 dark:text-gray-400"
      :title="rateHint"
      data-testid="intelligence-match-rate"
    >
      <span v-if="answerPreview">{{ answerPreview }} | </span>{{ rateLabel }}
    </p>
    <div class="flex max-w-full gap-0.5 py-0.5" :aria-label="t(key + 'recent', { count: recent.length })">
      <button
        v-for="item in chronological"
        :key="item.id"
        type="button"
        class="h-3 w-1.5 min-w-[3px] shrink rounded-[2px] transition-opacity hover:opacity-75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 dark:focus-visible:ring-offset-dark-800"
        :class="intelligenceColor(item.status)"
        :title="tooltip(item)"
        :aria-label="tooltip(item)"
        data-testid="intelligence-history-bar"
        @click="$emit('open')"
      />
    </div>
    <p class="max-w-full truncate text-gray-500 dark:text-gray-400" :title="modelHint" data-testid="intelligence-model">{{ latest.model }}</p>
    <time
      class="tabular-nums text-gray-500 dark:text-gray-400"
      :datetime="latest.finished_at || latest.created_at"
      :title="t(key + (latest.finished_at ? 'finished' : 'started'))"
      data-testid="intelligence-time"
    >{{ date(latest.finished_at || latest.created_at) }}</time>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { intelligenceColor, type AccountIntelligenceTest } from '@/api/admin/accountIntelligence'

const props = defineProps<{ items: AccountIntelligenceTest[] }>()
defineEmits<{ open: [] }>()
const { t, locale } = useI18n()
const key = 'admin.accounts.intelligenceSummary.'
const manual = 'admin.accounts.manualIntelligence.'

const recent = computed(() => [...props.items]
  .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at) || b.id.localeCompare(a.id))
  .slice(0, 20))
const chronological = computed(() => [...recent.value].reverse())
const latest = computed(() => recent.value[0])
const graded = computed(() => {
  const current = latest.value
  if (!current?.expected_answer || current.kind !== 'candy') return []
  return recent.value.filter(item =>
    item.kind === current.kind && item.model === current.model &&
    item.reasoning_effort === current.reasoning_effort && item.prompt === current.prompt &&
    item.scoring_version === current.scoring_version && item.expected_answer === current.expected_answer &&
    (item.status === 'correct' || item.status === 'incorrect'))
})
const rateLabel = computed(() => {
  if (latest.value?.kind === 'custom') return t(key + 'unscored')
  if (!graded.value.length) return t(key + 'noScoredResults')
  const rate = (graded.value.filter(item => item.status === 'correct').length / graded.value.length * 100).toFixed(1)
  return t(key + 'matchRate', { rate, count: graded.value.length })
})
const rateHint = computed(() => t(key + 'rateHint', { count: recent.value.length }))
const resultLabel = computed(() => {
  const item = latest.value
  if (!item) return ''
  if (item.status === 'correct' || item.status === 'incorrect') {
    return t(key + item.status, { answer: item.expected_answer || '—' })
  }
  return t(key + 'result', { status: t(manual + item.status) })
})
const resultColor = computed(() => ({
  correct: 'text-emerald-600 dark:text-emerald-400',
  incorrect: 'text-amber-700 dark:text-amber-400',
  error: 'text-red-600 dark:text-red-400',
  running: 'text-blue-600 dark:text-blue-400',
  completed: 'text-gray-600 dark:text-gray-400',
}[latest.value?.status || 'completed']))
const answerPreview = computed(() => {
  const item = latest.value
  if (!item || !['correct', 'incorrect'].includes(item.status)) return ''
  const text = item.response.trim().replace(/\s+/g, ' ')
  return text.length <= 12 ? text : ''
})
const modelHint = computed(() => {
  const item = latest.value
  return item ? [item.model, item.reasoning_effort, item.upstream_model && item.upstream_model !== item.model ? item.upstream_model : ''].filter(Boolean).join(' · ') : ''
})
const date = (value: string) => {
  const at = new Date(value)
  return Number.isNaN(at.getTime()) ? '—' : at.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-GB', { hour12: false })
}
const tooltip = (item: AccountIntelligenceTest) => [
  `${date(item.finished_at || item.created_at)} · ${item.model} · ${t('admin.accounts.intelligenceSchedule.' + (item.source === 'scheduled' ? 'automatic' : 'manual'))} · ${t(manual + item.status)}`,
  item.expected_answer ? `${t(manual + 'expected')}：${item.expected_answer}` : '',
  `${t(manual + 'question')}：${item.prompt}`,
  `${t(manual + 'answer')}：${item.error || item.response || '…'}`,
].filter(Boolean).join('\n')
</script>
