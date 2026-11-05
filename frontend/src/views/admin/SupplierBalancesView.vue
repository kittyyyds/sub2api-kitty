<template>
  <AppLayout>
    <div class="min-w-0 space-y-5">
      <header class="flex flex-wrap justify-between gap-3"><div><h1 class="text-2xl font-semibold">{{ t('financeHub.supplierTitle') }}</h1><p class="mt-2 text-sm text-gray-500">{{ t('financeHub.supplierDescription') }}</p></div><button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('financeHub.refresh') }}</button></header>
      <SmartOpsNav />
      <p class="rounded-xl bg-primary-50 p-4 text-sm leading-6 dark:bg-dark-800">{{ t('financeHub.balanceNote') }}</p>
      <form class="card space-y-3 p-4" @submit.prevent="save">
        <h2 class="font-semibold">{{ t('financeHub.thresholds') }}</h2><p class="text-sm text-gray-500">{{ t('financeHub.thresholdNote') }}</p>
        <div class="grid gap-3 sm:grid-cols-3"><label>{{ t('financeHub.currency') }}<input v-model="unit" class="input w-full" maxlength="3" @change="selectCurrency" /></label><label>{{ t('financeHub.wallet') }}<input v-model="walletLimit" class="input w-full" type="number" min="0" max="1000000000000" step="any" /></label><label>{{ t('financeHub.remaining') }}<input v-model="keyLimit" class="input w-full" type="number" min="0" max="1000000000000" step="any" /></label></div>
        <button class="btn btn-primary" :disabled="saving || !snapshot">{{ t('financeHub.save') }}</button><span class="ml-3 text-sm" role="status">{{ message }}</span>
      </form>
      <p v-if="error" role="alert" class="text-red-600">{{ error }}</p><p v-if="loading">{{ t('financeHub.loading') }}</p>
      <template v-if="snapshot">
        <div class="card overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th v-for="column in columns" :key="column" class="p-3">{{ t(`financeHub.${column}`) }}</th></tr></thead><tbody>
          <tr v-for="row in snapshot.rows" :key="row.account_id" class="border-t dark:border-dark-700" :class="row.alerts.length ? 'bg-amber-50 dark:bg-amber-950/30' : ''">
            <td class="p-3">{{ row.name }}<div class="text-xs text-gray-500">#{{ row.account_id }} · {{ row.platform }}</div></td>
            <td class="p-3 break-all">{{ row.host || t('financeHub.unknown') }}<div class="text-xs text-gray-500">{{ row.source }}</div></td>
            <td class="p-3">{{ amount(row.wallet, row.unit) }}<div v-if="row.alerts.includes('wallet')" class="text-amber-700">{{ t('financeHub.lowWallet') }}</div></td>
            <td class="p-3">{{ row.unlimited ? t('financeHub.unlimited') : amount(row.remaining, row.unit) }}<div v-if="row.alerts.includes('remaining')" class="text-amber-700">{{ t('financeHub.lowRemaining') }}</div></td>
            <td class="p-3">{{ t(`financeHub.states.${row.status}`) }}</td><td class="p-3 whitespace-nowrap">{{ date(row.received_at) }}<div class="text-xs text-gray-500">{{ date(row.fresh_until) }}</div></td>
          </tr>
        </tbody></table><p v-if="!snapshot.rows.length" class="p-5">{{ t('financeHub.empty') }}</p></div>
        <footer class="flex flex-wrap items-center justify-between gap-3 text-sm"><span>{{ t('financeHub.total') }}: {{ snapshot.total }} · {{ t('financeHub.page') }} {{ page }}</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="loading || page <= 1" @click="go(-1)">{{ t('financeHub.previous') }}</button><button class="btn btn-secondary" :disabled="loading || page * snapshot.page_size >= snapshot.total" @click="go(1)">{{ t('financeHub.next') }}</button></div></footer>
      </template>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import SmartOpsNav from '@/components/admin/operations/SmartOpsNav.vue'
import { readSupplierBalances, saveSupplierThresholds, type SupplierBalances } from '@/api/admin/supplierBalances'
const { t } = useI18n()
const columns = ['account', 'source', 'wallet', 'remaining', 'status', 'observed']
const snapshot = ref<SupplierBalances | null>(null), loading = ref(false), error = ref(''), page = ref(1)
const unit = ref('USD'), walletLimit = ref<string | number>(''), keyLimit = ref<string | number>(''), saving = ref(false), message = ref('')
let sequence = 0, alive = true, controller: AbortController | undefined
function amount(n: number | null, currency: string) { return n == null || !Number.isFinite(n) ? t('financeHub.unknown') : `${n.toLocaleString(undefined, { maximumFractionDigits: 6 })} ${currency || t('financeHub.unknown')}` }
function date(value: string | null) { const parsed = value ? new Date(value) : null; return parsed && Number.isFinite(parsed.getTime()) ? parsed.toLocaleString() : t('financeHub.unknown') }
function selectCurrency() { unit.value = unit.value.trim().toUpperCase(); const v = snapshot.value?.thresholds[unit.value]; walletLimit.value = v?.wallet ?? ''; keyLimit.value = v?.remaining ?? '' }
async function load() {
  const id = ++sequence; controller?.abort(); controller = new AbortController(); loading.value = true; error.value = ''; snapshot.value = null
  try { const next = await readSupplierBalances(page.value, controller.signal); if (alive && sequence === id) { snapshot.value = next; selectCurrency() } }
  catch { if (alive && sequence === id) error.value = t('financeHub.failed') }
  finally { if (alive && sequence === id) loading.value = false }
}
function go(delta: number) { page.value += delta; void load() }
async function save() {
  if (!snapshot.value) return
  const currency = unit.value.trim().toUpperCase(), wallet = walletLimit.value === '' ? null : Number(walletLimit.value), remaining = keyLimit.value === '' ? null : Number(keyLimit.value)
  if (!/^[A-Z]{3}$/.test(currency) || [wallet, remaining].some(n => n !== null && (!Number.isFinite(n) || n < 0 || n > 1e12))) { message.value = t('financeHub.invalid'); return }
  saving.value = true; message.value = ''
  try { await saveSupplierThresholds({ ...snapshot.value.thresholds, [currency]: { wallet, remaining } }); if (alive) { message.value = t('financeHub.saved'); await load() } }
  catch { if (alive) message.value = t('financeHub.failed') }
  finally { if (alive) saving.value = false }
}
onMounted(load)
onUnmounted(() => { alive = false; sequence++; controller?.abort() })
</script>
