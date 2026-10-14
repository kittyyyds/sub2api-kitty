<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { scheduledTestsAPI } from '@/api/admin/scheduledTests'
import { getIQSummaries, type IQSummary } from '@/api/admin/iqInterval'
import type { ScheduledTestPlan } from '@/types'
import IQHistoryStrip from './IQHistoryStrip.vue'
const props=defineProps<{accountId:number;defaultModel:string;reasoning:string;disabled?:boolean}>()
const plan=ref<ScheduledTestPlan|null>(null),summary=ref<IQSummary>(),interval=ref(60)
const enabled=ref(false),busy=ref(false),error=ref(''),legacy=ref<ScheduledTestPlan[]>([])
let disposed=false,historyBusy=false
const historyError=ref(false)
async function history(){if(historyBusy||disposed)return;historyBusy=true;try{const rows=await getIQSummaries([props.accountId]);if(!disposed){summary.value=rows[0];historyError.value=false}}catch{if(!disposed)historyError.value=true}finally{historyBusy=false}}
async function load(){busy.value=true;try{const plans=await scheduledTestsAPI.listByAccount(props.accountId);if(disposed)return;plan.value=plans.find(p=>p.pelican_config?.schedule_mode==='completion_interval')??null;legacy.value=plans.filter(p=>p.enabled&&p.pelican_config&&!p.pelican_config.quality&&!p.pelican_config.schedule_mode);if(plan.value){enabled.value=plan.value.enabled;interval.value=plan.value.pelican_config?.interval_seconds??60;}await history()}catch{error.value='读取定时设置失败，请重试'}finally{busy.value=false}}
async function save(){if(busy.value)return;busy.value=true;error.value='';try{const body={account_id:props.accountId,model_id:props.defaultModel,enabled:enabled.value,cron_expression:'* * * * *',max_results:20,auto_recover:false,pelican_config:{schedule_mode:'completion_interval' as const,interval_seconds:Number(interval.value),question_kind:'candy' as const,prompt:'fixed candy',reasoning_effort:props.reasoning||'medium',parallel_count:1}};plan.value=plan.value?await scheduledTestsAPI.update(plan.value.id,body):await scheduledTestsAPI.create(body);await history()}catch(e){error.value=(e as Error).message||'保存失败'}finally{busy.value=false}}
async function pauseLegacy(){busy.value=true;error.value='';try{for(const p of legacy.value)await scheduledTestsAPI.update(p.id,{enabled:false});await load()}catch{error.value='暂停旧计划失败，请重试'}finally{busy.value=false}}
onMounted(()=>void load());const timer=setInterval(()=>void history(),2000);onBeforeUnmount(()=>{disposed=true;clearInterval(timer)})
</script>
<template>
 <section class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
  <h3 class="font-semibold">后台定时智商测试</h3>
  <p class="text-sm text-gray-500">固定糖果题，沿用现有答案判定。每次完成后等待指定秒数再测；单账号不重叠，本站最多8个定时测试并行，超出排队。关闭网页仍继续，不自动禁用或调整账号。现有账号不会自动启用，需保存并开启。</p>
  <p v-if="legacy.length" class="rounded bg-amber-50 p-3 text-sm text-amber-900">发现 {{legacy.length}} 个旧式定时计划，仍按原设置运行。<button class="underline" :disabled="busy" @click="pauseLegacy">暂停旧计划（保留记录）</button></p>
  <p v-if="error" role="alert" class="text-sm text-red-600">{{error}}</p>
  <div class="grid gap-3 sm:grid-cols-2"><div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700">保存后使用上方模型：<b>{{defaultModel}}</b><br />思考强度：{{reasoning||'medium'}}<p class="mt-1 text-xs text-gray-500">不使用修改后的题目或手动并行数。首次保存后等待一个间隔再开始。</p></div><label><span class="input-label">每次完成后等待（秒）</span><input v-model.number="interval" type="number" min="1" max="86400" class="input" :disabled="busy||disabled" /></label></div>
  <label class="flex gap-2"><input v-model="enabled" type="checkbox" :disabled="busy||disabled" />启用定时测试</label>
  <IQHistoryStrip :summary="summary" :error="historyError" />
  <p class="text-xs text-gray-500">灰：未测试；绿：匹配21；黄：29或其他答案；红：请求失败。仅显示最近20次定时结果，手动测试记录单独保留。</p>
  <button class="btn btn-primary" :disabled="busy||disabled||!defaultModel.trim()" @click="save">保存定时设置</button>
 </section>
</template>
