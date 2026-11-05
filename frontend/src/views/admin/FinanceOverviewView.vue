<template>
  <AppLayout><div class="min-w-0 space-y-5">
    <header><h1 class="text-2xl font-semibold">{{ t('financeHub.businessTitle') }}</h1><p class="mt-2 text-sm text-gray-500">{{ t('financeHub.businessDescription') }}</p></header><SmartOpsNav />
    <p class="rounded-xl bg-primary-50 p-4 text-sm dark:bg-dark-800">{{ t('financeHub.globalScope') }}</p>
    <form class="card grid items-end gap-3 p-4 sm:grid-cols-3" @submit.prevent="apply"><label>{{ t('financeHub.from') }}<input v-model="from" type="datetime-local" required class="input w-full" /></label><label>{{ t('financeHub.until') }}<input v-model="until" type="datetime-local" required class="input w-full" /></label><button class="btn btn-primary" :disabled="loading">{{ t('financeHub.apply') }}</button></form>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p><p v-if="loading">{{ t('financeHub.loading') }}</p>
    <template v-if="snapshot">
      <section class="card space-y-3 p-4"><h2 class="font-semibold">{{ t('financeHub.valuation') }}</h2><p class="text-2xl">{{ number(snapshot.usage.valuation) }} USD</p><p class="text-sm text-gray-500">{{ t('financeHub.valuationNote') }}</p><p v-if="snapshot.usage.invalid_records" class="text-amber-700">{{ t('financeHub.partial') }} ({{ snapshot.usage.invalid_records }})</p><RouterLink to="/admin/profit-operations" class="text-primary-600 underline">{{ t('financeHub.costLink') }}</RouterLink><p class="text-sm text-gray-500">{{ t('financeHub.costNote') }}</p></section>
      <section class="card space-y-3 p-4"><h2 class="font-semibold">{{ t('financeHub.cashTitle') }}</h2><p class="text-sm text-gray-500">{{ t('financeHub.cashNote') }}</p><div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th v-for="key in cashColumns" :key="key" class="p-2">{{ t(`financeHub.${key}`) }}</th></tr></thead><tbody><tr v-for="r in snapshot.cash" :key="`${r.currency}:${r.order_type}`" class="border-t dark:border-dark-700"><td class="p-2">{{ r.currency }}</td><td class="p-2">{{ r.order_type }}</td><td class="p-2">{{ r.paid_orders }}</td><td class="p-2">{{ r.invalid_receipts ? t('financeHub.unknown') : number(r.received) }}</td><td class="p-2">{{ r.refunded_orders }}</td><td class="p-2">{{ number(r.refunded_credits) }}</td></tr></tbody></table><p v-if="!snapshot.cash.length">{{ t('financeHub.noRows') }}</p></div></section>
      <section class="grid gap-4 lg:grid-cols-2">
        <div class="card space-y-3 p-4"><h2 class="font-semibold">{{ t('financeHub.walletTitle') }}</h2><p>{{ t('financeHub.positive') }}: {{ number(snapshot.balances.positive) }}</p><p>{{ t('financeHub.negative') }}: {{ number(snapshot.balances.negative) }}</p><p v-if="snapshot.balances.invalid" class="text-amber-700">{{ t('financeHub.partial') }}</p><p class="text-sm text-gray-500">{{ t('financeHub.walletNote') }} {{ date(snapshot.queried_at) }}</p></div>
        <div class="card space-y-3 p-4"><h2 class="font-semibold">{{ t('financeHub.creditsTitle') }}</h2><p>{{ t('financeHub.affiliate') }}: {{ number(snapshot.credits.affiliate_transfers) }}</p><p>{{ t('financeHub.granted') }}: {{ number(snapshot.credits.recharge_bonus_granted) }}</p><p>{{ t('financeHub.revoked') }}: {{ number(snapshot.credits.recharge_bonus_revoked) }}</p><p class="text-sm text-gray-500">{{ t('financeHub.creditsNote') }}</p></div>
      </section>
      <section class="card space-y-3 p-4"><h2 class="font-semibold">{{ t('financeHub.monitorTitle') }}</h2><p class="text-sm text-gray-500">{{ t('financeHub.monitorNote') }}</p><div v-for="m in snapshot.monitoring" :key="m.source" class="rounded-lg border p-3 dark:border-dark-700"><strong>{{ t(`financeHub.sourceName.${m.source}`) }}</strong><p>{{ t('financeHub.samples') }}: {{ m.records }} · {{ t('financeHub.knownCost') }}: {{ number(m.recorded_cost) }} · {{ t('financeHub.unknownRecords') }}: {{ m.unknown }}</p></div></section>
    </template>
    <section class="card space-y-3 p-4"><h2 class="font-semibold">{{ t('financeHub.permanentTitle') }}</h2><p class="text-sm text-gray-500">{{ t('financeHub.permanentNote') }}</p><p class="text-sm text-gray-500">{{ t('financeHub.coverageNote') }}</p><p v-if="permanentError" role="alert" class="text-red-600">{{ permanentError }}</p>
      <template v-if="permanent"><p>{{ t('financeHub.captureStarted') }}: {{ date(permanent.state.started_at) }} · {{ t('financeHub.oldest') }}: {{ date(permanent.totals.oldest_observation) }}</p><p :class="permanent.state.baseline_complete ? 'text-green-700' : 'text-amber-700'">{{ t(permanent.state.baseline_complete ? 'financeHub.baselineReady' : 'financeHub.baselinePending') }}</p><button v-if="!permanent.state.baseline_complete" class="btn btn-secondary" :disabled="backfilling" @click="backfill">{{ t('financeHub.backfill') }}</button>
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"><div><p class="text-sm">{{ t('financeHub.requests') }}</p><strong class="text-xl">{{ number(permanent.totals.requests, 0) }}</strong></div><div><p class="text-sm">{{ t('financeHub.inputTokens') }}</p><strong class="text-xl">{{ number(permanent.totals.input_tokens, 0) }}</strong></div><div><p class="text-sm">{{ t('financeHub.outputTokens') }}</p><strong class="text-xl">{{ number(permanent.totals.output_tokens, 0) }}</strong></div><div><p class="text-sm">{{ t('financeHub.cacheTokens') }}</p><strong class="text-xl">{{ number(permanent.totals.cache_read_tokens + permanent.totals.cache_creation_tokens, 0) }}</strong></div></div><p>{{ t('financeHub.totalValuation') }}: {{ number(permanent.totals.recorded_valuation) }}</p><p v-if="permanent.totals.invalid_records" class="text-amber-700">{{ t('financeHub.partial') }} ({{ permanent.totals.invalid_records }})</p>
      </template>
    </section>
  </div></AppLayout>
</template>
<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import SmartOpsNav from '@/components/admin/operations/SmartOpsNav.vue'
import { readFinanceOverview, readPermanentUsage, backfillPermanentUsage, type FinanceOverview, type PermanentUsage } from '@/api/admin/financeOverview'
const { t } = useI18n(), cashColumns = ['currency', 'orderType', 'orders', 'received', 'refunds', 'refundedCredits']
const local = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
const from = ref(local(new Date(Date.now() - 86400000))), until = ref(local(new Date()))
const snapshot = ref<FinanceOverview | null>(null), permanent = ref<PermanentUsage | null>(null), loading = ref(false), error = ref(''), permanentError = ref(''), backfilling = ref(false)
let alive = true, sequence = 0, controller: AbortController | undefined
function number(n: number | null | undefined, decimals = 6) { return n == null || !Number.isFinite(n) ? t('financeHub.unknown') : n.toLocaleString(undefined, { maximumFractionDigits: decimals }) }
function date(value: string | null) { const d = value ? new Date(value) : null; return d && Number.isFinite(d.getTime()) ? d.toLocaleString() : t('financeHub.unknown') }
async function apply() {
  const a = new Date(from.value), b = new Date(until.value)
  if (!Number.isFinite(a.getTime()) || !Number.isFinite(b.getTime()) || b <= a || b.getTime() - a.getTime() > 31 * 86400000) { error.value = t('financeHub.rangeError'); return }
  const id = ++sequence; controller?.abort(); controller = new AbortController(); const signal = controller.signal
  snapshot.value = null; permanent.value = null; error.value = ''; permanentError.value = ''; loading.value = true
  await Promise.all([
    readFinanceOverview(a.toISOString(), b.toISOString(), signal).then(v => { if (alive && id === sequence) snapshot.value = v }).catch(() => { if (alive && id === sequence) error.value = t('financeHub.failed') }),
    readPermanentUsage(signal).then(v => { if (alive && id === sequence) permanent.value = v }).catch(() => { if (alive && id === sequence) permanentError.value = t('financeHub.failed') })
  ])
  if (alive && id === sequence) loading.value = false
}
async function backfill() { backfilling.value = true; try { await backfillPermanentUsage(); if (alive) await apply() } catch { if (alive) permanentError.value = t('financeHub.failed') } finally { if (alive) backfilling.value = false } }
onMounted(apply)
onUnmounted(() => { alive = false; sequence++; controller?.abort() })
</script>
