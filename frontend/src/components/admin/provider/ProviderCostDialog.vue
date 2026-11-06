<template>
  <BaseDialog :show="!!provider" :title="`${provider?.name ?? ''} · ${t('admin.upstream.costs.title')}`" width="wide" @close="!busy && emit('close')">
    <p v-if="error" role="alert" class="mb-4 text-sm text-red-600">{{ error }}</p>
    <p v-if="message" role="status" class="mb-4 text-sm text-emerald-600">{{ message }}</p>
    <p v-if="loading" class="py-8 text-center">{{ t('common.loading') }}</p>
    <template v-else-if="wallet && state">
      <p class="mb-4 text-sm leading-6 text-gray-500">{{ t('admin.upstream.costs.hint') }}</p>
      <div class="mb-5 grid grid-cols-2 gap-3 rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800">
        <div>{{ t('admin.upstream.costs.bound') }}<p class="mt-1 font-semibold">{{ bindings.map(b => `#${b.account_id} ${b.account_name}`).join('、') || '—' }}</p></div>
        <div>{{ t('admin.upstream.costs.unitCost') }}<p class="mt-1 font-semibold">{{ wallet.credit_cost == null ? '—' : `${wallet.credit_cost.toFixed(6)} CNY / USD` }}</p></div>
      </div>
      <form class="mb-5 space-y-3 border-b border-gray-200 pb-5 dark:border-dark-700" @submit.prevent="bindAccounts">
        <label class="input-label">{{ t('admin.upstream.costs.bindExisting') }}</label>
        <select v-model="selectedAccounts" multiple class="input min-h-24" :aria-label="t('admin.upstream.costs.bindExisting')">
          <option v-for="account in availableAccounts" :key="account.id" :value="account.id">#{{ account.id }} · {{ account.name }}</option>
        </select>
        <button class="btn btn-secondary" :disabled="busy || !selectedAccounts.length">{{ t('admin.upstream.costs.bind') }}</button>
      </form>
      <details class="mb-5 rounded-xl border border-gray-200 p-4 dark:border-dark-700" open>
        <summary class="cursor-pointer font-medium">{{ t('admin.upstream.costs.purchase') }}</summary>
        <form class="mt-4 space-y-3" @submit.prevent="savePurchase">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <label><span class="input-label">{{ t('admin.upstream.costs.cash') }}</span><input v-model.number="purchase.cash" class="input" type="number" min="0" step="any" required /></label>
            <label><span class="input-label">{{ t('admin.upstream.costs.credits') }}</span><input v-model.number="purchase.credits" class="input" type="number" min="0" step="any" required /></label>
            <label><span class="input-label">{{ t('admin.upstream.costs.bonus') }}</span><input v-model.number="purchase.bonus" class="input" type="number" min="0" step="any" required /></label>
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label><span class="input-label">{{ t('admin.upstream.costs.time') }}</span><input v-model="purchase.time" class="input" type="datetime-local" required /></label>
            <label><span class="input-label">{{ t('admin.upstream.costs.reference') }}</span><input v-model.trim="purchase.reference" class="input" maxlength="256" required /></label>
          </div>
          <button class="btn btn-secondary" :disabled="busy">{{ t('admin.upstream.costs.savePurchase') }}</button>
        </form>
        <ul v-if="purchases.length" class="mt-4 space-y-2 border-t border-gray-100 pt-3 text-xs dark:border-dark-700">
          <li v-for="item in purchases" :key="item.id">{{ new Date(item.occurred_at).toLocaleString() }} · {{ item.cash }} {{ item.cash_currency }} → {{ item.credits }} + {{ item.bonus }} USD · {{ item.reference }}</li>
        </ul>
      </details>
      <details class="rounded-xl border border-gray-200 p-4 dark:border-dark-700" open>
        <summary class="cursor-pointer font-medium">{{ t('admin.upstream.costs.rule') }}</summary>
        <form class="mt-4 space-y-3" @submit.prevent="saveRule">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label><span class="input-label">{{ t('admin.upstream.costs.model') }}</span><input v-model.trim="rule.model" class="input" required maxlength="160" placeholder="*" /></label>
            <label><span class="input-label">{{ t('admin.upstream.costs.method') }}</span><select v-model="rule.method" class="input"><option v-for="method in methods" :key="method" :value="method">{{ t(`providerHub.methods.${method}`) }}</option></select></label>
          </div>
          <p v-if="rule.method === 'account_credits'" class="text-xs text-gray-500">{{ t('admin.upstream.costs.weightedHint') }}</p>
          <label v-if="rule.method === 'account_credits'" class="block"><span class="input-label">{{ t('admin.upstream.costs.fx') }}</span><input v-model.number="rule.fx_to_usd" class="input" type="number" min="0.000000001" step="any" required /></label>
          <label v-else-if="rule.method === 'standard_multiplier'" class="block"><span class="input-label">{{ t('admin.upstream.costs.multiplier') }}</span><input v-model.number="rule.multiplier" class="input" type="number" min="0" step="any" required /></label>
          <div v-else-if="rule.method === 'tokens'" class="grid grid-cols-2 gap-3">
            <label v-for="field in tokenFields" :key="field"><span class="input-label">{{ t(`providerHub.${{input_price:'inputPrice',output_price:'outputPrice',cache_read_price:'cacheReadPrice',cache_write_price:'cacheWritePrice'}[field]}`) }} · USD / 1M</span><input v-model.number="rule[field]" class="input" type="number" min="0" step="any" required /></label>
          </div>
          <label v-else class="block"><span class="input-label">{{ t('admin.upstream.costs.unitPrice') }}</span><input v-model.number="rule.unit_price" class="input" type="number" min="0" step="any" required /></label>
          <button class="btn btn-primary" :disabled="busy || (rule.method === 'account_credits' && wallet.credit_cost == null)">{{ t('admin.upstream.costs.saveRule') }}</button>
        </form>
        <p v-for="item in rules" :key="item.id" class="mt-3 text-xs text-gray-500">{{ item.model }} · {{ t(`providerHub.methods.${item.method}`) }} · {{ item.currency }} · {{ new Date(item.started_at).toLocaleString() }}</p>
      </details>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { Provider } from '@/api/admin/upstream'
import { getProviderHub, getProviderPurchases, providerHubWrite, type ProviderHubState, type SupplierPurchase } from '@/api/admin/providerHub'
const props = defineProps<{ provider: Provider | null }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const state = ref<ProviderHubState | null>(null), purchases = ref<SupplierPurchase[]>([])
const loading = ref(false), busy = ref(false), error = ref(''), message = ref('')
const selectedAccounts = ref<number[]>([])
const wallet = computed(() => state.value?.wallets.find(w => w.id === props.provider?.wallet_id))
const bindings = computed(() => state.value?.bindings.filter(b => b.wallet_id === wallet.value?.id) ?? [])
const availableAccounts = computed(() => state.value?.accounts.filter(a => !state.value?.bindings.some(b => b.account_id === a.id)) ?? [])
const rules = computed(() => state.value?.rules.filter(r => r.wallet_id === wallet.value?.id) ?? [])
const methods = ['account_credits', 'standard_multiplier', 'tokens', 'image', 'video_second', 'video_item', 'request']
const tokenFields = ['input_price', 'output_price', 'cache_read_price', 'cache_write_price'] as const
const purchase = reactive({ cash: 0, credits: 0, bonus: 0, reference: '', time: '', operation_id: '' })
const rule = reactive({ model: '*', method: 'account_credits', multiplier: 1, fx_to_usd: null as number | null, input_price: 0, output_price: 0, cache_read_price: 0, cache_write_price: 0, unit_price: 0 })
let sequence = 0
async function load() {
  const provider = props.provider, seq = ++sequence
  if (!provider) return
  loading.value = true
  try {
    const [next, page] = await Promise.all([getProviderHub(), getProviderPurchases(provider.wallet_id)])
    if (seq !== sequence || props.provider?.id !== provider.id) return
    state.value = next; purchases.value = page.items.filter(p => !p.voided_at)
  } catch { if (seq === sequence) error.value = t('providerHub.failed') }
  finally { if (seq === sequence) loading.value = false }
}
watch(() => props.provider?.id, () => {
  ++sequence; state.value = null; error.value = ''; message.value = ''; selectedAccounts.value = []
  Object.assign(rule, { model: '*', method: 'account_credits', multiplier: 1, fx_to_usd: null, input_price: 0, output_price: 0, cache_read_price: 0, cache_write_price: 0, unit_price: 0 })
  const now = new Date(); now.setMinutes(now.getMinutes() - now.getTimezoneOffset())
  Object.assign(purchase, { cash: 0, credits: 0, bonus: 0, reference: '', time: now.toISOString().slice(0, 16), operation_id: crypto.randomUUID() })
  if (props.provider) void load()
}, { immediate: true })
async function mutate(path: string, body: Record<string, unknown>, method: 'post' | 'put' = 'post') {
  if (busy.value) return false
  busy.value = true; error.value = ''; message.value = ''
  try { await providerHubWrite(path, body, method); message.value = t('admin.upstream.costs.saved'); await load(); return true }
  catch { error.value = t('admin.upstream.costs.failed'); return false }
  finally { busy.value = false }
}
async function bindAccounts() {
  if (!wallet.value) return
  if (await mutate(`/wallets/${wallet.value.id}/bindings`, { revision: wallet.value.revision, account_ids: selectedAccounts.value, reassign: false }, 'put')) selectedAccounts.value = []
}
async function savePurchase() {
  if (!wallet.value || purchase.cash + purchase.credits + purchase.bonus <= 0) return
  if (await mutate(`/wallets/${wallet.value.id}/purchases`, { ...purchase, kind: 'topup', currency: 'CNY', fx_rate: 1, occurred_at: new Date(purchase.time).toISOString() })) {
    purchase.operation_id = crypto.randomUUID(); purchase.cash = 0; purchase.credits = 0; purchase.bonus = 0; purchase.reference = ''
  }
}
async function saveRule() {
  if (!wallet.value) return
  await mutate(`/wallets/${wallet.value.id}/rules`, { ...rule, revision: wallet.value.revision, currency: rule.method === 'account_credits' ? 'CNY' : 'USD', fx_to_usd: rule.method === 'account_credits' ? rule.fx_to_usd : 1 })
}
</script>
