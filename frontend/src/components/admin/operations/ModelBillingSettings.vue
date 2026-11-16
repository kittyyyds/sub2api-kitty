<template>
  <section class="min-w-0 space-y-5 rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900" data-testid="model-billing">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-lg font-semibold">{{ t('billingRules.modelBilling.title') }}</h2>
      <label class="flex items-center gap-2 text-sm"><input v-model="config.enabled" data-testid="model-billing-enabled" type="checkbox" role="switch" />{{ t('billingRules.enable') }}</label>
    </header>
    <p class="text-sm leading-6 text-gray-500">{{ t('billingRules.modelBilling.hint') }}</p>
    <p class="rounded-xl bg-amber-50 p-3 text-sm leading-6 text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('billingRules.modelBilling.migration') }}</p>
    <p v-if="!config.rules.length" class="py-3 text-sm text-gray-500">{{ t('billingRules.modelBilling.empty') }}</p>
    <article v-for="(rule, index) in config.rules" :key="index" class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-700" :data-testid="'model-billing-rule-' + index">
      <header class="flex items-center justify-between gap-3">
        <label class="flex items-center gap-2 text-sm"><input v-model="rule.enabled" type="checkbox" :data-testid="'model-billing-rule-enabled-' + index" />{{ t('billingRules.enable') }} #{{ index + 1 }}</label>
        <button type="button" class="btn btn-secondary" :data-testid="'model-billing-remove-' + index" @click="config.rules.splice(index, 1)">{{ t('billingRules.modelBilling.remove') }}</button>
      </header>
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <label class="block min-w-0"><span class="mb-2 block text-sm">{{ t('billingRules.modelBilling.group') }}</span>
          <select v-model.number="rule.group_id" class="input w-full" :data-testid="'model-billing-group-' + index" @change="rule.model = ''; void loadModels(rule.group_id)">
            <option :value="0" disabled>{{ t('billingRules.modelBilling.chooseGroup') }}</option>
            <option v-for="g in groups" :key="g.id" :value="g.id" :disabled="g.status !== 'active'">{{ g.name }} · #{{ g.id }}{{ g.status !== 'active' ? ' (' + t('billingRules.modelBilling.inactive') + ')' : '' }}</option>
          </select>
        </label>
        <div class="min-w-0"><label :for="'model-billing-model-' + index" class="mb-2 block text-sm">{{ t('billingRules.modelBilling.model') }}</label>
          <div class="flex gap-2"><input v-model="rule.model" :id="'model-billing-model-' + index" :data-testid="'model-billing-model-' + index" :list="'billing-models-' + index" maxlength="200" required class="input min-w-0 flex-1" placeholder="gpt-6-luna" />
            <button type="button" class="btn btn-secondary shrink-0" :disabled="!rule.group_id || busy[rule.group_id]" @click="loadModels(rule.group_id)">{{ t('billingRules.modelBilling.fetch') }}</button></div>
          <datalist :id="'billing-models-' + index"><option v-for="m in models[rule.group_id] || []" :key="m" :value="m" /></datalist>
          <p v-if="errors[rule.group_id]" role="alert" class="mt-1 text-xs text-red-600">{{ t('billingRules.modelBilling.modelsFailed') }}</p>
        </div>
        <label><span class="mb-2 block text-sm">{{ t('billingRules.modelBilling.mode') }}</span><select v-model="rule.mode" class="input w-full" :data-testid="'model-billing-mode-' + index"><option value="context">{{ t('billingRules.modelBilling.context') }}</option><option value="always">{{ t('billingRules.modelBilling.always') }}</option></select></label>
        <label><span class="mb-2 block text-sm">{{ t('billingRules.modelBilling.multiplier') }}</span><div class="flex items-center gap-2"><input v-model.number="rule.multiplier" type="number" min="1" max="1000" step="any" required class="input w-full" :data-testid="'model-billing-multiplier-' + index" />×</div></label>
      </div>
      <label v-if="rule.mode === 'context'" class="block max-w-xs"><span class="mb-2 block text-sm">{{ t('billingRules.modelBilling.threshold') }}</span><div class="flex items-center gap-2"><input :value="rule.threshold_tokens / 1000" type="number" min="0.001" max="100000" step="0.001" required class="input w-full" :data-testid="'model-billing-threshold-' + index" @input="rule.threshold_tokens = Math.round(Number(($event.target as HTMLInputElement).value) * 1000)" /><span>K</span></div></label>
      <p class="rounded-lg bg-gray-50 p-3 text-sm leading-6 dark:bg-dark-800">{{ t(rule.mode === 'context' ? 'billingRules.modelBilling.previewContext' : 'billingRules.modelBilling.previewAlways', { group: groups.find(g => g.id === rule.group_id)?.name || '—', model: rule.model || '—', threshold: rule.threshold_tokens.toLocaleString(), multiplier: rule.multiplier }) }}</p>
    </article>
    <button type="button" data-testid="model-billing-add" class="btn btn-secondary" :disabled="config.rules.length >= 100" @click="config.rules.push(newModelBillingRule())">{{ t('billingRules.modelBilling.add') }}</button>
    <p class="text-xs leading-6 text-gray-500">{{ t('billingRules.modelBilling.matchHint') }}</p>
    <p class="text-xs leading-6 text-gray-500">{{ t('billingRules.modelBilling.example') }}</p>
  </section>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAvailableModels } from '@/api/admin/groups'
import { newModelBillingRule, type ModelBillingConfig } from '@/utils/modelBilling'
import type { Group } from '@/types'
defineProps<{ groups: Group[] }>()
const config = defineModel<ModelBillingConfig>({ required: true })
const { t } = useI18n()
const models = ref<Record<number, string[]>>({}), busy = ref<Record<number, boolean>>({}), errors = ref<Record<number, boolean>>({})
let alive = true
async function loadModels(id: number) {
  if (!id || busy.value[id]) return
  busy.value[id] = true; errors.value[id] = false
  try { const names = await getAvailableModels(id); if (alive) models.value[id] = names }
  catch { if (alive) errors.value[id] = true }
  finally { if (alive) busy.value[id] = false }
}
watch(() => config.value.rules.map(r => r.group_id), ids => { for (const id of new Set(ids)) if (id && !models.value[id] && !errors.value[id]) void loadModels(id) }, { immediate: true })
onBeforeUnmount(() => { alive = false })
</script>
