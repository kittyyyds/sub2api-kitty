<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { scheduledTestsAPI } from '@/api/admin/scheduledTests'
import { getIQSummaries,setIQQuarantineTarget, type IQSummary } from '@/api/admin/iqInterval'
import type { ScheduledTestPlan,Group } from '@/types'
import { getAllIncludingInactive } from '@/api/admin/groups'
import IQHistoryStrip from './IQHistoryStrip.vue'
const props=defineProps<{accountId:number;defaultModel:string;reasoning:string;disabled?:boolean}>()
const plan=ref<ScheduledTestPlan|null>(null),summary=ref<IQSummary>(),interval=ref(60)
const enabled=ref(false),busy=ref(false),error=ref(''),legacy=ref<ScheduledTestPlan[]>([])
let disposed=false,historyBusy=false
const historyError=ref(false)
const groups=ref<Group[]>([]),targetGroup=ref(0)
async function loadGroups(){try{groups.value=await getAllIncludingInactive()}catch{error.value='读取分组失败，请重试'}}
async function saveTarget(){if(!plan.value||!targetGroup.value)return;busy.value=true;error.value='';try{await setIQQuarantineTarget(plan.value.id,targetGroup.value);await history()}catch(e){error.value=(e as Error).message||'保存目标失败'}finally{busy.value=false}}
async function history(){if(historyBusy||disposed)return;historyBusy=true;try{const rows=await getIQSummaries([props.accountId]);if(!disposed){summary.value=rows[0];historyError.value=false}}catch{if(!disposed)historyError.value=true}finally{historyBusy=false}}
async function load(){busy.value=true;try{const plans=await scheduledTestsAPI.listByAccount(props.accountId);if(disposed)return;plan.value=plans.find(p=>p.pelican_config?.schedule_mode==='completion_interval')??null;legacy.value=plans.filter(p=>p.enabled&&p.pelican_config&&!p.pelican_config.quality&&!p.pelican_config.schedule_mode);if(plan.value){enabled.value=plan.value.enabled;interval.value=plan.value.pelican_config?.interval_seconds??60;}await history()}catch{error.value='读取定时设置失败，请重试'}finally{busy.value=false}}
async function save(){if(busy.value)return;busy.value=true;error.value='';try{const body={account_id:props.accountId,model_id:props.defaultModel,enabled:enabled.value,cron_expression:'* * * * *',max_results:20,auto_recover:false,pelican_config:{schedule_mode:'completion_interval' as const,interval_seconds:Number(interval.value),question_kind:'candy' as const,prompt:'fixed candy',reasoning_effort:props.reasoning||'medium',parallel_count:1}};plan.value=plan.value?await scheduledTestsAPI.update(plan.value.id,body):await scheduledTestsAPI.create(body);await history()}catch(e){error.value=(e as Error).message||'保存失败'}finally{busy.value=false}}
async function pauseLegacy(){busy.value=true;error.value='';try{for(const p of legacy.value)await scheduledTestsAPI.update(p.id,{enabled:false});await load()}catch{error.value='暂停旧计划失败，请重试'}finally{busy.value=false}}
onMounted(()=>{void load();void loadGroups()});const timer=setInterval(()=>void history(),2000);onBeforeUnmount(()=>{disposed=true;clearInterval(timer)})
</script>
<template>
 <section class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
  <h3 class="font-semibold">后台定时智商测试</h3>
  <p class="text-sm text-gray-500">固定糖果题，每次完成后等待指定秒数再测；单账号不重叠，本站最多8个定时测试并行，超出排队。关闭网页仍继续。未发出的等待不计作失败。</p>
  <div v-if="summary?.lifecycle" class="space-y-2 rounded-lg bg-blue-50 p-3 text-sm dark:bg-blue-950/30" data-testid="iq-lifecycle">
   <p v-if="summary.lifecycle.quarantined_at">已隔离到分组 #{{summary.lifecycle.config.quarantine_group_id}}，暂停自动升档。测试继续，答对不会自动恢复原分组。</p>
   <p v-else-if="summary.lifecycle.quarantine_pending">隔离待处理，客户调度关闭，自动升档冻结。重新选择有效目标后，下轮测试会完成隔离；不会自动恢复调度。</p>
   <p v-else-if="summary.lifecycle.passed_at">首次验证已通过，已执行升到 {{summary.lifecycle.config.passed_concurrency}} 并发并开放客户调度；后续由普通并发升级规则接管。</p>
   <p v-else>首次验证中：1 并发，客户调度关闭。连续正确 {{summary.lifecycle.correct_streak}} / {{summary.lifecycle.config.correct_required}} 次，达标才升到 {{summary.lifecycle.config.passed_concurrency}} 并发并打开调度。暂停测试不会解除验证门槛。</p>
   <p v-if="summary.lifecycle.config.quarantine_enabled">连续回答 29：{{summary.lifecycle.answer29_streak}} / 6</p>
   <div v-if="!summary.lifecycle.quarantined_at" class="space-y-2">
    <label class="text-xs">设置此账号的隔离目标<select v-model.number="targetGroup" class="input" :disabled="busy"><option :value="0">请选择已有分组</option><option v-for="g in groups" :key="g.id" :value="g.id" :disabled="g.platform!=='openai'||g.status!=='active'">{{g.name}} · #{{g.id}} · {{g.platform}} · {{g.status}}</option></select></label>
    <button class="btn btn-secondary" :disabled="busy||!targetGroup" @click="saveTarget">保存目标并启用 29 隔离规则</button>
    <p class="text-xs text-gray-500">仅更新本账号。首次验证进度不变；隔离待处理时不会自动放开调度。</p>
   </div>
   <p v-if="summary.lifecycle.action_error" role="alert" class="text-red-600">{{summary.lifecycle.action_error}}</p>
  </div>
  <p v-else class="text-xs text-gray-500">此账号未加入自动验证流程，定时测试仅观察，不自动调整账号。</p>
  <p v-if="legacy.length" class="rounded bg-amber-50 p-3 text-sm text-amber-900">发现 {{legacy.length}} 个旧式定时计划，仍按原设置运行。<button class="underline" :disabled="busy" @click="pauseLegacy">暂停旧计划（保留记录）</button></p>
  <p v-if="error" role="alert" class="text-sm text-red-600">{{error}}</p>
  <div class="grid gap-3 sm:grid-cols-2"><div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700">保存后使用上方模型：<b>{{defaultModel}}</b><br />思考强度：{{reasoning||'medium'}}<p class="mt-1 text-xs text-gray-500">不使用修改后的题目或手动并行数。首次保存后等待一个间隔再开始。</p></div><label><span class="input-label">每次完成后等待（秒）</span><input v-model.number="interval" type="number" min="1" max="86400" class="input" :disabled="busy||disabled" /></label></div>
  <label class="flex gap-2"><input v-model="enabled" type="checkbox" :disabled="busy||disabled" />启用定时测试</label>
  <IQHistoryStrip :summary="summary" :error="historyError" />
  <p class="text-xs text-gray-500">灰：未测试；绿：匹配21；黄：29或其他答案；红：请求失败。仅显示最近20次定时结果，手动测试记录单独保留。</p>
  <button class="btn btn-primary" :disabled="busy||disabled||!defaultModel.trim()" @click="save">保存定时设置</button>
 </section>
</template>
