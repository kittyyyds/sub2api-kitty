<script setup lang="ts">
import { computed } from 'vue'
import type { IQSummary } from '@/api/admin/iqInterval'
const props=defineProps<{summary?:IQSummary;error?:boolean}>()
const cells=computed(()=>{const rows=props.summary?.results.slice(0,20).reverse()??[];return [...Array.from({length:20-rows.length},()=>null),...rows]})
const correct=computed(()=>props.summary?.results.filter(r=>r.verdict==='correct').length??0)
const state=computed(()=>!props.summary?'未设置':!props.summary.enabled?'已暂停':props.summary.running_until&&Date.parse(props.summary.running_until)>Date.now()?'测试中':props.summary.next_run_at&&Date.parse(props.summary.next_run_at)>Date.now()?`等待 ${Math.ceil((Date.parse(props.summary.next_run_at)-Date.now())/1000)}s`:'等待 / 排队')
</script>
<template>
 <div class="mt-1.5 min-w-[160px] max-w-[220px]" data-testid="iq-history-strip">
  <div class="flex gap-0.5" aria-label="最近20次定时测试，由左到右从旧到新">
   <span v-for="(r,i) in cells" :key="r?.id??`empty-${i}`" class="h-3 min-w-0 flex-1 rounded-sm" :class="!r?'bg-gray-300 dark:bg-dark-500':r.verdict==='correct'?'bg-emerald-500':r.verdict==='error'?'bg-red-500':'bg-amber-400'" :title="!r?'未测试':`${r.model} / ${r.reasoning} · ${new Date(r.finished_at).toLocaleString()} · ${r.error||r.answer}`" :data-verdict="r?.verdict??'untested'"></span>
  </div>
  <p v-if="error" class="mt-1 text-[10px] text-red-600">记录读取失败，未将失败当作未测试</p>
  <p v-else class="mt-1 text-[10px] text-gray-500">{{state}} · {{correct}}/{{summary?.results.length??0}} 匹配 · {{summary?.results.length?Math.round(correct/summary.results.length*100):0}}% <span v-if="summary">· 间隔 {{summary.interval_seconds}}s</span></p>
  <p v-if="summary" class="truncate text-[10px] text-gray-500" :title="summary.model">{{summary.model}} / {{summary.reasoning}}<span v-if="summary.results[0]"> · {{new Date(summary.results[0].finished_at).toLocaleTimeString()}}</span></p>
 </div>
</template>
