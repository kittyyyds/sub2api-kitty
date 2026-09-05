<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-5 pb-8">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div>
          <div class="flex items-center gap-2">
            <span class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300"><Icon name="shield" size="sm" /></span>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white">模型能力与身份审计</h1>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">汇总真实流量证据，并对账号映射后的模型执行受控主动探测。能力支持与身份可信度分开判断。</p>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新审计结果" @click="refreshAll"><Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />刷新记录</button>
          <button type="button" class="btn btn-primary" :disabled="running || loading" title="批量探测启用账号和已观测模型" @click="runBatchProbe"><Icon name="play" size="sm" />{{ running ? '探测中…' : '一键探测' }}</button>
        </div>
      </header>

      <section class="grid gap-4 lg:grid-cols-2">
        <div class="border-l-4 border-sky-500 bg-sky-50 px-4 py-3 dark:bg-sky-950/20"><div class="flex items-center gap-2 text-sm font-semibold text-sky-900 dark:text-sky-200"><Icon name="database" size="sm" />被动真实流量证据</div><p class="mt-1 text-xs leading-5 text-sky-800/80 dark:text-sky-200/70">来自已完成请求的请求模型、映射模型和上游响应模型，不产生额外上游请求。</p></div>
        <div class="border-l-4 border-violet-500 bg-violet-50 px-4 py-3 dark:bg-violet-950/20"><div class="flex items-center gap-2 text-sm font-semibold text-violet-900 dark:text-violet-200"><Icon name="beaker" size="sm" />主动受控探测</div><p class="mt-1 text-xs leading-5 text-violet-800/80 dark:text-violet-200/70">对启用的 OpenAI 兼容账号发送短请求，记录协议、工具调用、状态码和延迟。</p></div>
      </section>

      <section class="flex flex-wrap items-end gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <label class="block min-w-32"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">观察窗口</span><select v-model.number="days" class="input w-full" :disabled="running" @change="refreshAll"><option :value="7">最近 7 天</option><option :value="30">最近 30 天</option><option :value="90">最近 90 天</option></select></label>
        <label class="flex h-9 items-center gap-2 px-1 text-sm text-gray-700 dark:text-gray-200"><input v-model="activeOnly" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500" :disabled="running" />仅探测启用账号</label>
        <label class="block w-28"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">探测上限</span><input v-model.number="maxItems" type="number" min="1" max="240" class="input w-full" :disabled="running" /></label>
        <span v-if="lastRun" class="pb-2 text-xs text-gray-500 dark:text-gray-400">上次运行 #{{ lastRun.id }}：{{ runStatusLabel(lastRun.status) }}，{{ lastRun.completed_count }}/{{ lastRun.total_count }} 个账号模型</span>
      </section>

      <section class="grid grid-cols-2 gap-3 md:grid-cols-5"><div v-for="card in summaryCards" :key="card.label" class="border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800"><p class="text-xs text-gray-500 dark:text-gray-400">{{ card.label }}</p><p class="mt-1 text-2xl font-semibold" :class="card.color">{{ card.value }}</p></div></section>

      <section class="border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-dark-700"><div><h2 class="text-sm font-semibold text-gray-900 dark:text-white">模型审计矩阵</h2><p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">点击行查看协议证据、映射链和最近响应模型</p></div><span class="text-xs text-gray-500 dark:text-gray-400">{{ summary?.generated_at ? formatTime(summary.generated_at) : '尚未加载' }}</span></div>
        <div v-if="loading" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">正在读取审计记录…</div>
        <div v-else-if="rows.length === 0" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">暂无证据。先积累真实请求，或点击“一键探测”生成主动记录。</div>
        <div v-else class="overflow-x-auto"><table class="min-w-[980px] w-full text-left text-sm"><thead class="bg-gray-50 text-xs font-semibold text-gray-500 dark:bg-dark-900/50 dark:text-gray-400"><tr><th class="px-4 py-3">账号 / 分组</th><th class="px-4 py-3">请求模型</th><th class="px-4 py-3">映射模型</th><th class="px-4 py-3">协议能力</th><th class="px-4 py-3">身份判定</th><th class="px-4 py-3">证据</th><th class="px-4 py-3">最近活动</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="row in rows" :key="row.key" class="cursor-pointer transition hover:bg-gray-50 dark:hover:bg-dark-900/40" @click="selectedRow = row"><td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ row.account_name || `账号 #${row.account_id}` }}</div><div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ row.platform }} · {{ row.group_name || '未分组' }}</div></td><td class="px-4 py-3 font-mono text-xs text-gray-800 dark:text-gray-200">{{ row.requested_model }}</td><td class="px-4 py-3 font-mono text-xs text-gray-600 dark:text-gray-300">{{ row.mapped_model || '直通' }}</td><td class="px-4 py-3"><div class="flex flex-wrap gap-1"><span v-if="Object.keys(row.protocols).length === 0" class="text-xs text-gray-400">未主动探测</span><span v-for="(evidence, protocol) in row.protocols" :key="protocol" class="rounded px-1.5 py-0.5 text-[11px]" :class="statusClass(evidence.status)" :title="evidence.error || evidence.evidence">{{ protocolShort(protocol) }} {{ protocolStatus(evidence.status) }}</span></div><div class="mt-1 text-xs" :class="capabilityClass(row.capability_status)">{{ capabilityLabel(row.capability_status) }}</div></td><td class="px-4 py-3"><span class="rounded px-2 py-1 text-xs font-medium" :class="identityClass(row.identity_status)">{{ identityLabel(row.identity_status) }}</span></td><td class="px-4 py-3 text-xs text-gray-600 dark:text-gray-300"><div>主动 {{ Object.keys(row.protocols).length }} 条</div><div>真实 {{ row.passive_requests }} 次<span v-if="row.passive_mismatches" class="ml-1 text-amber-600">· 偏差 {{ row.passive_mismatches }}</span></div></td><td class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400">{{ formatTime(row.active_last_seen || row.passive_last_seen) }}</td></tr></tbody></table></div>
      </section>

      <section class="border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"><div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">探测历史</h2></div><div v-if="runs.length === 0" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">还没有主动探测记录</div><div v-else class="overflow-x-auto"><table class="min-w-[720px] w-full text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/50 dark:text-gray-400"><tr><th class="px-4 py-2.5">运行</th><th class="px-4 py-2.5">状态</th><th class="px-4 py-2.5">进度</th><th class="px-4 py-2.5">结果</th><th class="px-4 py-2.5">时间</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="run in runs" :key="run.id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-900/40" @click="openRun(run.id)"><td class="px-4 py-2.5 font-mono text-xs">#{{ run.id }}</td><td class="px-4 py-2.5"><span class="rounded px-2 py-1 text-xs" :class="runStatusClass(run.status)">{{ runStatusLabel(run.status) }}</span></td><td class="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{{ run.completed_count }}/{{ run.total_count }}</td><td class="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">通过 {{ run.verified_count }} · 偏差 {{ run.mismatch_count }} · 不支持 {{ run.unsupported_count }} · 异常 {{ run.error_count }}</td><td class="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">{{ formatTime(run.created_at) }}</td></tr></tbody></table></div></section>
      <div v-if="errorMessage" class="border-l-4 border-red-500 bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/20 dark:text-red-200">{{ errorMessage }}</div>
    </div>

    <div v-if="selectedRow" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="selectedRow = null"><section class="max-h-[90vh] w-full max-w-3xl overflow-y-auto border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"><div class="flex items-start justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700"><div><h2 class="font-semibold text-gray-900 dark:text-white">{{ selectedRow.requested_model }}</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ selectedRow.account_name || `账号 #${selectedRow.account_id}` }} · {{ selectedRow.platform }} · {{ selectedRow.group_name || '未分组' }}</p></div><button type="button" class="icon-btn" title="关闭详情" @click="selectedRow = null"><Icon name="x" size="sm" /></button></div><div class="grid gap-3 border-b border-gray-200 p-5 text-sm sm:grid-cols-3 dark:border-dark-700"><div><span class="text-xs text-gray-500">能力</span><p class="mt-1 font-medium" :class="capabilityClass(selectedRow.capability_status)">{{ capabilityLabel(selectedRow.capability_status) }}</p></div><div><span class="text-xs text-gray-500">身份</span><p class="mt-1 font-medium" :class="identityTextClass(selectedRow.identity_status)">{{ identityLabel(selectedRow.identity_status) }} · {{ selectedRow.confidence }}</p></div><div><span class="text-xs text-gray-500">真实流量</span><p class="mt-1 font-medium text-gray-900 dark:text-white">{{ selectedRow.passive_requests }} 次 / 偏差 {{ selectedRow.passive_mismatches }}</p></div></div><div class="p-5"><h3 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500">协议证据</h3><div class="space-y-2"><div v-for="(evidence, protocol) in selectedRow.protocols" :key="protocol" class="border border-gray-200 p-3 dark:border-dark-700"><div class="flex flex-wrap items-center justify-between gap-2"><span class="font-medium text-gray-900 dark:text-white">{{ protocolLabel(protocol) }}</span><span class="rounded px-2 py-1 text-xs" :class="statusClass(evidence.status)">{{ protocolStatus(evidence.status) }}</span></div><div class="mt-2 grid gap-2 text-xs text-gray-600 sm:grid-cols-3 dark:text-gray-300"><span>HTTP {{ evidence.http_status || '-' }}</span><span>{{ evidence.latency_ms }} ms</span><span>返回 {{ evidence.upstream_model || '-' }}</span></div><p class="mt-2 text-xs text-gray-500 dark:text-gray-400">证据：{{ evidence.evidence }}<span v-if="evidence.error"> · {{ evidence.error }}</span></p></div><p v-if="Object.keys(selectedRow.protocols).length === 0" class="text-sm text-gray-500">尚无主动探测证据</p></div><div v-if="selectedRow.passive_response_models?.length" class="mt-5"><h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">真实响应模型</h3><div class="flex flex-wrap gap-2"><span v-for="model in selectedRow.passive_response_models" :key="model" class="rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ model }}</span></div></div></div></section></div>

    <div v-if="runDetail" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="runDetail = null"><section class="max-h-[85vh] w-full max-w-2xl overflow-y-auto border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"><div class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700"><h2 class="font-semibold text-gray-900 dark:text-white">探测运行 #{{ runDetail.run.id }}</h2><button type="button" class="icon-btn" title="关闭详情" @click="runDetail = null"><Icon name="x" size="sm" /></button></div><div class="space-y-2 p-5"><div v-for="(item, index) in runDetail.evidence" :key="`${item.checked_at}-${index}`" class="border-b border-gray-100 py-2 text-xs dark:border-dark-700"><div class="flex justify-between gap-3"><span class="font-medium">{{ protocolLabel(item.protocol) }}</span><span :class="statusTextClass(item.status)">{{ protocolStatus(item.status) }}</span></div><div class="mt-1 text-gray-500">HTTP {{ item.http_status || '-' }} · {{ item.latency_ms }} ms · {{ item.upstream_model || '未返回模型' }} · {{ formatTime(item.checked_at) }}</div><div v-if="item.error" class="mt-1 text-red-600">{{ item.error }}</div></div></div></section></div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { CapabilityAuditRow, CapabilityAuditRun, CapabilityAuditRunDetail, CapabilityAuditSummary } from '@/api/admin/modelCapabilityAudit'

const summary = ref<CapabilityAuditSummary | null>(null)
const runs = ref<CapabilityAuditRun[]>([])
const lastRun = ref<CapabilityAuditRun | null>(null)
const selectedRow = ref<CapabilityAuditRow | null>(null)
const runDetail = ref<CapabilityAuditRunDetail | null>(null)
const days = ref(30)
const maxItems = ref(60)
const activeOnly = ref(true)
const loading = ref(false)
const running = ref(false)
const errorMessage = ref('')
const rows = computed(() => summary.value?.rows ?? [])
const summaryCards = computed(() => [
  { label: '审计模型', value: summary.value?.total_models ?? 0, color: 'text-gray-900 dark:text-white' },
  { label: '能力已验证', value: summary.value?.capability_verified ?? 0, color: 'text-emerald-600' },
  { label: '身份一致', value: summary.value?.identity_consistent ?? 0, color: 'text-sky-600' },
  { label: '疑似替换', value: summary.value?.mismatch_suspected ?? 0, color: 'text-amber-600' },
  { label: '未验证', value: summary.value?.unverified ?? 0, color: 'text-gray-500' },
])
function formatTime(value?: string | null): string { if (!value) return '-'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
function protocolShort(value: string): string { return value === 'chat_completions' ? 'Chat' : value === 'responses_tools' ? 'Tools' : 'Resp' }
function protocolLabel(value: string): string { return value === 'chat_completions' ? 'Chat Completions' : value === 'responses_tools' ? 'Responses + Tools' : value === 'responses' ? 'Responses' : value }
function protocolStatus(value: string): string { return value === 'verified' ? '通过' : value === 'model_mismatch' ? '偏差' : value === 'unsupported' ? '不支持' : value === 'timeout' ? '超时' : value === 'error' ? '异常' : '待复测' }
function statusClass(value: string): string { return value === 'verified' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300' : value === 'model_mismatch' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300' : value === 'unsupported' ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300' }
function statusTextClass(value: string): string { return value === 'verified' ? 'text-emerald-600' : value === 'model_mismatch' ? 'text-amber-600' : 'text-red-600' }
function capabilityLabel(value: string): string { return value === 'verified' ? '能力支持' : value === 'unsupported' ? '能力不支持' : '能力未验证' }
function capabilityClass(value: string): string { return value === 'verified' ? 'text-emerald-600' : value === 'unsupported' ? 'text-gray-500' : 'text-amber-600' }
function identityLabel(value: string): string { return value === 'consistent' ? '身份一致' : value === 'mismatch_suspected' ? '疑似模型替换' : value === 'evidence_only' ? '仅有能力证据' : '未验证' }
function identityClass(value: string): string { return value === 'consistent' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300' : value === 'mismatch_suspected' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' }
function identityTextClass(value: string): string { return value === 'consistent' ? 'text-emerald-600' : value === 'mismatch_suspected' ? 'text-amber-600' : 'text-gray-600' }
function runStatusLabel(value: string): string { return value === 'completed' ? '已完成' : value === 'partial' ? '部分完成' : value === 'running' ? '运行中' : value === 'failed' ? '失败' : value }
function runStatusClass(value: string): string { return value === 'completed' ? 'bg-emerald-100 text-emerald-700' : value === 'partial' ? 'bg-amber-100 text-amber-700' : value === 'running' ? 'bg-blue-100 text-blue-700' : 'bg-red-100 text-red-700' }
async function refreshAll() { loading.value = true; errorMessage.value = ''; try { const [nextSummary, nextRuns] = await Promise.all([adminAPI.modelCapabilityAudit.getSummary(days.value), adminAPI.modelCapabilityAudit.listRuns()]); summary.value = nextSummary; runs.value = nextRuns; if (!lastRun.value && nextRuns[0]) lastRun.value = nextRuns[0] } catch (error) { errorMessage.value = extractApiErrorMessage(error) } finally { loading.value = false } }
async function runBatchProbe() { running.value = true; errorMessage.value = ''; try { lastRun.value = await adminAPI.modelCapabilityAudit.runProbes({ active_only: activeOnly.value, max_items: Math.max(1, Math.min(240, Number(maxItems.value) || 60)) }); await refreshAll() } catch (error) { errorMessage.value = extractApiErrorMessage(error) } finally { running.value = false } }
async function openRun(id: number) { try { runDetail.value = await adminAPI.modelCapabilityAudit.getRun(id) } catch (error) { errorMessage.value = extractApiErrorMessage(error) } }
onMounted(refreshAll)
</script>
