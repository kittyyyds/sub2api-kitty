<template>
 <BaseDialog :show="true" :title="t('admin.accounts.manualIntelligence.title')" width="wide" @close="$emit('close')">
  <div class="space-y-4">
   <p class="font-semibold">#{{ account.id }} · {{ account.name }}</p>
   <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.manualIntelligence.hint') }}</p>
   <div class="grid gap-3 sm:grid-cols-3">
    <label class="text-sm">{{ t('admin.accounts.manualIntelligence.question') }}<select v-model="kind" class="input mt-1 w-full"><option value="candy">{{ t('admin.accounts.manualIntelligence.candy') }}</option><option value="custom">{{ t('admin.accounts.manualIntelligence.custom') }}</option></select></label>
    <label class="text-sm">{{ t('admin.accounts.manualIntelligence.model') }}<input v-model="model" class="input mt-1 w-full" maxlength="200" :list="`iq-models-${account.id}`" /><datalist :id="`iq-models-${account.id}`"><option v-for="name in models" :key="name" :value="name" /></datalist></label>
    <label class="text-sm">{{ t('admin.accounts.manualIntelligence.reasoning') }}<select v-model="reasoning" class="input mt-1 w-full"><option value="">{{ t('admin.accounts.manualIntelligence.default') }}</option><option v-for="level in ['none','minimal','low','medium','high','xhigh','max','ultra']" :key="level" :value="level">{{ level }}</option></select></label>
   </div>
   <textarea v-if="kind === 'custom'" v-model="prompt" class="input w-full" rows="4" maxlength="16000" :placeholder="t('admin.accounts.manualIntelligence.prompt')" />
   <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
   <div class="flex flex-wrap gap-2"><button type="button" class="btn btn-primary" :disabled="submitting || running || !model.trim() || (kind === 'custom' && !prompt.trim())" @click="start">{{ t(running ? 'admin.accounts.manualIntelligence.running' : 'admin.accounts.manualIntelligence.start') }}</button><button type="button" class="btn btn-secondary" :disabled="loading" @click="refresh">{{ t('common.refresh') }}</button><button type="button" class="btn btn-secondary" @click="$emit('schedule')">{{ t('admin.accounts.intelligenceSchedule.title') }}</button></div>
   <div class="flex items-center gap-2 text-sm"><h3 class="font-semibold">{{ t('admin.accounts.manualIntelligence.history') }}</h3><span class="text-xs text-gray-500">{{ t('admin.accounts.manualIntelligence.legend') }}</span></div>
   <p v-if="!items.length && !loading" class="text-sm text-gray-500">{{ t('admin.accounts.manualIntelligence.empty') }}</p>
   <div class="max-h-[48vh] space-y-2 overflow-y-auto">
    <details v-for="item in items" :key="item.id" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" :open="item.id === newestID">
     <summary class="cursor-pointer text-sm"><span class="mr-2 inline-block h-3 w-3 rounded-sm" :class="intelligenceColor(item.status)" />{{ new Date(item.created_at).toLocaleString() }} · {{ item.model }} · {{ t('admin.accounts.intelligenceSchedule.' + (item.source === 'scheduled' ? 'automatic' : 'manual')) }} · {{ t('admin.accounts.manualIntelligence.' + item.status) }} · {{ (item.latency_ms / 1000).toFixed(2) }}s</summary>
     <dl class="mt-3 space-y-2 text-sm"><dt class="text-gray-500">{{ t('admin.accounts.manualIntelligence.question') }} · {{ item.scoring_version }}<span v-if="item.expected_answer"> · {{ t('admin.accounts.manualIntelligence.expected') }} {{ item.expected_answer }}</span></dt><dd class="whitespace-pre-wrap break-words">{{ item.prompt }}</dd><dt class="text-gray-500">{{ t('admin.accounts.manualIntelligence.answer') }}<span v-if="item.upstream_model"> · {{ item.upstream_model }}</span></dt><dd class="whitespace-pre-wrap break-words" :class="item.error ? 'text-red-600 dark:text-red-400' : ''">{{ item.error || item.response || '…' }}</dd></dl>
    </details>
   </div>
   <button v-if="hasMore" type="button" class="btn btn-secondary" :disabled="loading" @click="loadMore">{{ t('admin.accounts.manualIntelligence.more') }}</button>
  </div>
 </BaseDialog>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountListItem } from '@/types'
import { accountIntelligence, intelligenceColor, type AccountIntelligenceTest } from '@/api/admin/accountIntelligence'
const props = defineProps<{ account: AccountListItem }>()
const emit = defineEmits<{ close: []; updated: []; schedule: [] }>()
const { t } = useI18n()
const kind = ref<'candy' | 'custom'>('candy'), prompt = ref(''), reasoning = ref('')
const models = computed(() => Object.keys(props.account.credentials?.model_mapping || {}).filter(name => !/[\*]/.test(name)))
const model = ref(models.value[0] || (props.account.platform === 'anthropic' ? 'claude-sonnet-4-6' : 'gpt-6-astra'))
const items = ref<AccountIntelligenceTest[]>([]), error = ref(''), loading = ref(false), submitting = ref(false), hasMore = ref(false), newestID = ref('')
const running = computed(() => items.value.some(item => item.status === 'running'))
let alive = true, timer: ReturnType<typeof setTimeout> | undefined
let requestID: string | undefined
const message = (e: any) => e.response?.data?.message || t('admin.accounts.manualIntelligence.failed')
async function refresh() {
 if (loading.value) return
 loading.value = true
 try { const result = await accountIntelligence.history(props.account.id); if (alive) { const older = items.value.slice(50).filter(item => !result.some(v => v.id === item.id)); items.value = [...result, ...older]; if (!older.length) hasMore.value = result.length === 50; error.value = ''; emit('updated') } }
 catch (e) { if (alive) error.value = message(e) }
 finally { loading.value = false; clearTimeout(timer); if (alive) timer = setTimeout(refresh, running.value ? 2000 : 10000) }
}
async function start() {
 if (submitting.value || running.value) return
 submitting.value = true; error.value = ''; requestID ||= crypto.randomUUID()
 try { const result = await accountIntelligence.start(props.account.id, { request_id: requestID, kind: kind.value, model: model.value.trim(), reasoning_effort: reasoning.value, prompt: prompt.value }); if (!alive) return; requestID = undefined; newestID.value = result.id; items.value = [result, ...items.value.filter(v => v.id !== result.id)]; emit('updated'); clearTimeout(timer); timer = setTimeout(refresh, 2000) }
 catch (e) { if (alive) { error.value = message(e); if ((e as any).response) requestID = undefined } }
 finally { submitting.value = false }
}
async function loadMore() {
 if (loading.value) return
 loading.value = true
 try { const result = await accountIntelligence.history(props.account.id, items.value.length); if (alive) { items.value.push(...result.filter(v => !items.value.some(i => i.id === v.id))); hasMore.value = result.length === 50 } }
 catch (e) { if (alive) error.value = message(e) }
 finally { loading.value = false }
}
onMounted(refresh)
onBeforeUnmount(() => { alive = false; clearTimeout(timer) })
</script>
