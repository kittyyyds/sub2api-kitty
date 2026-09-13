<script setup lang="ts">
import type { CandyRun } from '@/api/candyTests'
import { candyStatusLabels, candyEffortLabels } from '@/api/candyTests'
defineProps<{ run: CandyRun; expanded?: boolean }>()
const time = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })
const tokens = (value: number | null) => value == null ? '未返回' : value.toLocaleString()
</script>

<template>
  <section class="candy-result">
    <div class="result-line">
      <span class="verdict" :class="run.status">{{ candyStatusLabels[run.status] }}</span>
      <span>{{ run.finished_at ? `${(run.latency_ms / 1000).toFixed(1)} 秒` : '等待本轮完成' }}</span>
    </div>
    <div v-if="run.answer || run.standard_answer" class="answer-block"><template v-if="run.answer"><span>模型答案</span><strong>{{ run.answer }}</strong></template><span v-if="run.standard_answer">本轮标准答案：{{ run.standard_answer }}</span></div>
    <p v-if="run.message" class="result-message">{{ run.message }}</p>
    <div class="question-block"><span>本轮题目</span><p>{{ run.question }}</p></div>
    <div class="response-block">
      <div class="text-caption">模型回答原文</div>
      <pre :class="{ expanded }">{{ run.output || (['queued', 'running'].includes(run.status) ? '等待接口返回完整回答…' : '本轮没有可展示的回答') }}</pre>
    </div>
    <div class="token-row">
      <span>输入 {{ tokens(run.input_tokens) }}</span><span>输出 {{ tokens(run.output_tokens) }}</span><span>缓存命中 {{ tokens(run.cache_read_tokens) }}</span>
    </div>
    <div class="record-footer">
      <span>测试日期：{{ time(run.created_at) }}<template v-if="run.finished_at"> · 完成于 {{ time(run.finished_at) }}</template></span><span>{{ run.model }} · {{ candyEffortLabels[run.reasoning_effort] || run.reasoning_effort }}</span>
    </div>
    <p v-if="expanded && run.response_model" class="record-note">接口返回的型号标签：{{ run.response_model }}（仅作记录）</p>
  </section>
</template>

<style scoped>
.candy-result { display: grid; gap: 16px; color: var(--candy-text, #1e293b); }
.result-line,.token-row,.record-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; font-size: 12px; color: var(--candy-muted, #64748b); }
.verdict { display: inline-flex; align-items: center; gap: 7px; font-size: 13px; font-weight: 650; }
.verdict::before { content: ''; width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
.passed { color: #059669; }.failed,.error { color: #e05252; }.inconclusive { color: #b7791f; }.running,.queued { color: #0284c7; }
.answer-block { display: flex; gap: 16px; align-items: center; background: var(--candy-tint, #f0fdfa); border: 1px solid var(--candy-border, #dce9e5); padding: 15px 18px; border-radius: 12px; }
.answer-block span { font-size: 12px; white-space: nowrap; color: var(--candy-muted, #64748b); }.answer-block strong { font-size: 23px; font-weight: 650; overflow-wrap: anywhere; }
.question-block { font-size: 13px; line-height: 1.75; }.question-block > span,.text-caption { font-size: 11px; font-weight: 650; color: var(--candy-muted, #64748b); }.question-block p { margin-top: 5px; white-space: pre-wrap; overflow-wrap: anywhere; max-height: 180px; overflow: auto; }
.response-block { border: 1px solid var(--candy-border, #e2e8f0); background: var(--candy-well, #f8fafc); border-radius: 12px; overflow: hidden; }
.text-caption { padding: 10px 15px; border-bottom: 1px solid var(--candy-border, #e2e8f0); }
pre { margin: 0; min-height: 120px; max-height: 250px; overflow: auto; padding: 17px; font: inherit; font-size: 14px; line-height: 1.85; white-space: pre-wrap; overflow-wrap: anywhere; }pre.expanded { max-height: 55vh; }
.token-row { justify-content: flex-start; gap: 18px; font-variant-numeric: tabular-nums; }.record-footer { border-top: 1px solid var(--candy-border, #e2e8f0); padding-top: 12px; }
.result-message,.record-note { font-size: 12px; color: var(--candy-muted, #64748b); line-height: 1.7; }.result-message { padding: 10px 12px; background: var(--candy-well, #f8fafc); border-radius: 8px; }
</style>
