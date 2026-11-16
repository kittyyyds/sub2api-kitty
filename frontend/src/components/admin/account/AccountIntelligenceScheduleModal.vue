<template>
 <BaseDialog :show="true" :title="t(key + 'title')" width="wide" @close="$emit('close')">
  <form class="space-y-4" @submit.prevent="save">
   <p class="font-semibold">#{{ account.id }} · {{ account.name }}</p>
   <p class="text-sm text-gray-500 dark:text-gray-400">{{ t(key + 'hint') }}</p>
   <p v-if="loading" role="status">{{ t('common.loading') }}</p>
   <template v-else-if="loaded">
    <div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800" role="status">
     {{ t(key + (saved?.enabled ? 'active' : 'paused')) }}
     <span v-if="saved?.next_run_at"> · {{ t(key + 'next') }} {{ date(saved.next_run_at) }}</span>
     <p v-if="saved?.last_attempt_at">{{ t(key + 'last') }} {{ date(saved.last_attempt_at) }}</p>
     <p v-if="saved?.last_error" class="text-red-600 dark:text-red-400">{{ saved.last_error }}</p>
     <p v-if="saved?.timezone" class="mt-1 text-xs text-gray-500">{{ t(key + 'timezone') }} {{ saved.timezone }}</p>
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
     <label class="text-sm">{{ t(iq + 'question') }}<select v-model="form.kind" data-testid="schedule-kind" class="input mt-1 w-full"><option value="candy">{{ t(iq + 'candy') }}</option><option value="custom">{{ t(iq + 'custom') }}</option></select></label>
     <label class="text-sm">{{ t(iq + 'model') }}<input v-model="form.model" class="input mt-1 w-full" data-testid="schedule-model" required maxlength="200" :list="`schedule-models-${account.id}`" /><datalist :id="`schedule-models-${account.id}`"><option v-for="m in models" :key="m.value" :value="m.value">{{ m.label }}</option></datalist></label>
     <label class="text-sm">{{ t(iq + 'reasoning') }}<select v-model="form.reasoning_effort" class="input mt-1 w-full"><option value="">{{ t(iq + 'default') }}</option><option v-for="level in ['none','minimal','low','medium','high','xhigh','max','ultra']" :key="level" :value="level">{{ level }}</option></select></label>
     <label class="text-sm">{{ t(key + 'period') }}<select v-model="period" data-testid="schedule-period" class="input mt-1 w-full"><option v-for="preset in presets" :key="preset.value" :value="preset.value">{{ t(key + preset.label) }}</option><option value="custom">{{ t(key + 'customCron') }}</option></select></label>
     <label v-if="period === 'custom'" class="text-sm">{{ t(key + 'cron') }}<input v-model="form.cron_expression" data-testid="schedule-cron" class="input mt-1 w-full font-mono" placeholder="*/30 * * * *" maxlength="100" /><span class="mt-1 block text-xs text-gray-500">{{ t(key + 'cronHint') }}</span></label>
     <label class="text-sm">{{ t(key + 'keep') }}<input v-model.number="form.max_results" data-testid="schedule-keep" type="number" min="1" max="1000" class="input mt-1 w-full" required /><span class="mt-1 block text-xs text-gray-500">{{ t(key + 'keepHint') }}</span></label>
    </div>
    <label v-if="form.kind === 'custom'" class="block text-sm">{{ t(iq + 'prompt') }}<textarea v-model="form.prompt" class="input mt-1 w-full" rows="4" maxlength="16000" required /></label>
    <label class="flex items-center gap-2 text-sm"><input v-model="form.enabled" data-testid="schedule-enabled" type="checkbox" />{{ t(key + 'enable') }}</label>
    <p v-if="success" role="status" class="text-sm text-emerald-600 dark:text-emerald-400">{{ t(key + 'saved') }}</p>
   </template>
   <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
   <div class="flex justify-end gap-2">
    <button type="button" class="btn btn-secondary" :disabled="saving || loading" @click="load">{{ t('common.refresh') }}</button>
    <button type="submit" data-testid="schedule-save" class="btn btn-primary" :disabled="!loaded || loading || saving">{{ t(saving ? 'common.saving' : 'common.save') }}</button>
   </div>
  </form>
 </BaseDialog>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountListItem } from '@/types'
import { accountIntelligence, type AccountIntelligenceSchedule } from '@/api/admin/accountIntelligence'
const props = defineProps<{account: AccountListItem; modelOptions?: {value: string; label: string}[]}>()
const emit = defineEmits<{close: []; updated: []}>()
const { t } = useI18n(), key = 'admin.accounts.intelligenceSchedule.', iq = 'admin.accounts.manualIntelligence.'
const defaultCron = '*/30 * * * *'
const presets = [
 {value: '* * * * *', label: 'every1'},
 {value: defaultCron, label: 'every30'},
 {value: '0 * * * *', label: 'hourly'},
 {value: '0 */6 * * *', label: 'every6h'},
 {value: '0 0 * * *', label: 'daily'},
]
const period = ref(defaultCron), loading = ref(false), loaded = ref(false), saving = ref(false), error = ref(''), success = ref(false)
const saved = ref<AccountIntelligenceSchedule | null>(null)
const form = reactive<AccountIntelligenceSchedule>({account_id: props.account.id, revision:0, enabled:false, kind:'candy', model:'', reasoning_effort:'', prompt:'', cron_expression:defaultCron, max_results:100, timezone:'', next_run_at:null, last_attempt_at:null, last_error:''})
const models = computed(() => {
 const options = props.modelOptions?.length ? props.modelOptions : Object.keys(props.account.credentials?.model_mapping || {}).filter(v => !v.includes('*')).map(value => ({value,label:value}))
 return options.filter(v => !/image|video|audio|tts|whisper/i.test(v.value))
})
let alive = true
const date = (value: string) => new Date(value).toLocaleString()
const message = (e: any) => e.response?.data?.message || t(key + 'failed')
async function load() {
 if (loading.value || saving.value) return
 loading.value = true; error.value = ''; success.value = false; loaded.value = false
 try {
  const result = await accountIntelligence.schedule(props.account.id)
  if (!alive) return
  saved.value = {...result}; Object.assign(form,result)
  if (!form.model) form.model = models.value[0]?.value || (props.account.platform === 'anthropic' ? 'claude-sonnet-4-6' : 'gpt-6-astra')
  period.value = presets.some(preset => preset.value === result.cron_expression) ? result.cron_expression : 'custom'
  loaded.value = true
 } catch (e) {if (alive) error.value = message(e)}
 finally {if (alive) loading.value = false}
}
async function save() {
 if (!loaded.value || loading.value || saving.value) return
 error.value = ''; success.value = false
 const cron = (period.value === 'custom' ? form.cron_expression : period.value).trim()
 if (cron.split(/\s+/).length !== 5 || !/^[\d\s*,/\-]+$/.test(cron)) {error.value = t(key + 'invalidCron'); return}
 if (!form.model.trim() || (form.kind === 'custom' && !form.prompt.trim())) {error.value = t(key + 'required');return}
 if (!Number.isInteger(form.max_results) || form.max_results < 1 || form.max_results > 1000) {error.value = t(key + 'invalidKeep');return}
 saving.value = true
 try {
  const result = await accountIntelligence.saveSchedule(props.account.id,{...form,model:form.model.trim(),cron_expression:cron})
  if (!alive) return
  saved.value = {...result}; Object.assign(form,result); success.value = true; emit('updated')
 } catch (e) {if (alive) error.value = message(e)}
 finally {if (alive) saving.value = false}
}
onMounted(load)
onBeforeUnmount(() => {alive = false})
</script>
