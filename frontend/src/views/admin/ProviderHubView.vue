<template>
  <AppLayout>
    <div class="min-w-0 space-y-5">
      <header class="flex flex-wrap items-center justify-between gap-3">
        <h1 class="text-2xl font-semibold">{{ t('providerHub.title') }}</h1>
        <div class="flex flex-wrap gap-2">
          <RouterLink class="btn btn-secondary" to="/admin/operations"><Icon name="chart" size="sm" />{{ t('providerHub.operationsLink') }}</RouterLink>
          <button class="btn btn-secondary" :disabled="loading || busy" :title="t('providerHub.refresh')" :aria-label="t('providerHub.refresh')" @click="load"><Icon name="refresh" size="sm" /></button>
          <button class="btn btn-primary" :disabled="busy" @click="open('supplier')"><Icon name="plus" size="sm" />{{ t('providerHub.newSupplier') }}</button>
        </div>
      </header>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="message" role="status" class="text-sm text-emerald-700 dark:text-emerald-400">{{ message }}</p>
      <p v-if="loading && !state" role="status">{{ t('providerHub.loading') }}</p>
      <div v-if="state" class="grid min-w-0 gap-5 xl:grid-cols-[220px_minmax(0,1fr)]">
        <aside class="min-w-0 space-y-3 border-b border-gray-200 pb-4 xl:border-b-0 xl:border-r xl:pr-4 dark:border-dark-700">
          <label class="flex items-center gap-2 text-xs text-gray-500"><input v-model="showArchived" type="checkbox" />{{ t('providerHub.showArchived') }}</label>
          <button v-for="item in suppliers" :key="item.id" class="flex w-full items-start justify-between gap-2 rounded-md px-3 py-2 text-left text-sm" :class="supplierId === item.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'hover:bg-gray-100 dark:hover:bg-dark-800'" @click="chooseSupplier(item.id)">
            <span class="min-w-0 break-words">{{ item.name }}<span v-if="!item.active" class="block text-xs text-gray-500">{{ t('providerHub.archived') }}</span></span><span class="text-xs text-gray-400">{{ state.wallets.filter(w => w.supplier_id === item.id).length }}</span>
          </button>
          <p v-if="!suppliers.length" class="py-4 text-sm text-gray-500">{{ t('providerHub.emptySuppliers') }}</p>
        </aside>
        <main v-if="supplier" class="min-w-0 space-y-5">
          <section class="flex flex-wrap justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
            <div class="min-w-0"><h2 class="text-lg font-semibold break-words">{{ supplier.name }}</h2><div class="mt-1 break-all text-sm text-gray-500">{{ supplier.base_url || t('providerHub.manual') }}</div><p v-if="supplier.notes" class="mt-2 whitespace-pre-wrap break-words text-sm">{{ supplier.notes }}</p></div>
            <div class="flex items-start gap-2"><button class="icon-button" :title="t('providerHub.editSupplier')" :aria-label="t('providerHub.editSupplier')" :disabled="!supplier.active || busy" @click="open('supplier', supplier)"><Icon name="edit" size="sm" /></button><button class="icon-button" :title="t('providerHub.archive')" :aria-label="t('providerHub.archive')" :disabled="!supplier.active || busy" @click="confirmAction(`/suppliers/${supplier.id}/archive`, { revision: supplier.revision }, 'confirmArchive')"><Icon name="trash" size="sm" /></button><button class="btn btn-primary" :disabled="!supplier.active || busy" @click="open('wallet')"><Icon name="plus" size="sm" />{{ t('providerHub.newWallet') }}</button></div>
          </section>
          <div class="table-scroll"><table class="hub-table"><thead><tr><th>{{ t('providerHub.wallet') }}</th><th>{{ t('providerHub.balance') }}</th><th>{{ t('providerHub.creditCost') }}</th><th>{{ t('providerHub.updated') }}</th><th>{{ t('providerHub.actions') }}</th></tr></thead><tbody>
            <tr v-for="w in wallets" :key="w.id" :class="walletId === w.id ? 'bg-primary-50/60 dark:bg-primary-950/20' : ''">
              <td><button class="text-left font-medium text-primary-600 hover:underline dark:text-primary-300" @click="walletId = w.id; remoteKeys = []">{{ w.name }}</button><div class="text-xs text-gray-500">{{ w.identity }} · {{ w.has_secret ? t('providerHub.configured') : t('providerHub.notConfigured') }}</div></td>
              <td class="tabular-nums">{{ amount(w.balance, w.balance_unit || w.credit_currency) }}<div v-if="w.balance != null && w.low_balance != null && w.balance <= w.low_balance && w.fresh && !w.sync_error" class="text-xs text-red-600">{{ t('providerHub.low') }}</div><div v-if="!w.fresh && w.observed_at" class="text-xs text-amber-600">{{ t('providerHub.stale') }}</div><div v-if="w.sync_error" class="text-xs text-red-600" :title="w.sync_error">{{ t('providerHub.syncFailure') }}</div></td>
              <td class="tabular-nums">{{ amount(w.credit_cost, w.currency) }}</td><td class="whitespace-nowrap text-xs">{{ date(w.observed_at) }}</td>
              <td><div class="flex gap-1"><button class="icon-button" :title="t('providerHub.sync')" :aria-label="t('providerHub.sync')" :disabled="busy || !w.active" @click="syncWallet(w)"><Icon name="sync" size="sm" /></button><button class="icon-button" :title="t('providerHub.editWallet')" :aria-label="t('providerHub.editWallet')" :disabled="busy || !w.active" @click="open('wallet', w)"><Icon name="edit" size="sm" /></button><button class="icon-button" :title="t('providerHub.authorization')" :aria-label="t('providerHub.authorization')" :disabled="busy || !w.active" @click="open('authorization', w)"><Icon name="cog" size="sm" /></button><button class="icon-button" :title="t('providerHub.archive')" :aria-label="t('providerHub.archive')" :disabled="busy || !w.active" @click="confirmAction(`/wallets/${w.id}/archive`, { revision: w.revision }, 'confirmArchive')"><Icon name="trash" size="sm" /></button></div></td>
            </tr>
            <tr v-if="!wallets.length"><td colspan="5">{{ t('providerHub.empty') }}</td></tr>
          </tbody></table></div>
          <template v-if="wallet">
            <nav class="flex flex-wrap gap-1 border-b border-gray-200 dark:border-dark-700" :aria-label="t('providerHub.wallet')"><button v-for="item in tabs" :key="item" class="border-b-2 px-4 py-3 text-sm" :class="tab === item ? 'border-primary-500 font-medium text-primary-700 dark:text-primary-300' : 'border-transparent text-gray-500'" @click="tab = item">{{ t(`providerHub.${item}`) }}</button></nav>
            <section v-if="tab === 'accounts'" class="space-y-4">
              <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ wallet.name }} · {{ t('providerHub.accounts') }}</h3><button class="btn btn-primary" :disabled="busy || !wallet.active" @click="open('bindings')"><Icon name="link" size="sm" />{{ t('providerHub.bind') }}</button></div>
              <div class="table-scroll"><table class="hub-table"><thead><tr><th>ID</th><th>{{ t('providerHub.account') }}</th><th>{{ t('providerHub.started') }}</th><th>{{ t('providerHub.actions') }}</th></tr></thead><tbody><tr v-for="binding in bindings" :key="binding.id"><td>{{ binding.account_id }}</td><td>{{ binding.account_name || t('providerHub.unknown') }}</td><td>{{ date(binding.started_at) }}</td><td><button class="btn btn-secondary" :disabled="busy" @click="confirmAction(`/bindings/${binding.id}/unbind`, { revision: wallet.revision }, 'confirmUnbind')">{{ t('providerHub.unbind') }}</button></td></tr><tr v-if="!bindings.length"><td colspan="4">{{ t('providerHub.empty') }}</td></tr></tbody></table></div>
            </section>
            <section v-if="tab === 'purchases'" class="space-y-4">
              <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ wallet.name }} · {{ t('providerHub.purchases') }}</h3><button class="btn btn-primary" :disabled="busy || !wallet.active" @click="open('purchase')"><Icon name="plus" size="sm" />{{ t('providerHub.purchase') }}</button></div><p class="text-xs text-gray-500">{{ t('providerHub.creditsNote') }}</p>
              <div class="table-scroll"><table class="hub-table"><thead><tr><th>{{ t('providerHub.occurredAt') }}</th><th>{{ t('providerHub.kind') }}</th><th>{{ t('providerHub.cash') }}</th><th>{{ t('providerHub.credits') }}</th><th>{{ t('providerHub.bonus') }}</th><th>{{ t('providerHub.reference') }}</th><th>{{ t('providerHub.actions') }}</th></tr></thead><tbody><tr v-for="p in purchases" :key="p.id" :class="p.voided_at ? 'text-gray-400 line-through' : ''"><td class="whitespace-nowrap">{{ date(p.occurred_at) }}</td><td>{{ t(`providerHub.${p.kind}`) }}</td><td>{{ amount(p.cash, p.cash_currency) }}</td><td>{{ amount(p.credits, wallet.credit_currency) }}</td><td>{{ p.bonus }}</td><td class="max-w-48 break-all" :title="p.note">{{ p.reference || '—' }}</td><td><span v-if="p.voided_at">{{ t('providerHub.voided') }}</span><button v-else class="btn btn-secondary" :disabled="busy" @click="confirmAction(`/purchases/${p.id}/void`, { revision: p.revision }, 'confirmVoid')">{{ t('providerHub.void') }}</button></td></tr><tr v-if="!purchases.length"><td colspan="7">{{ purchasesLoading ? t('providerHub.loading') : t('providerHub.empty') }}</td></tr></tbody></table></div>
              <div class="flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500"><span>{{ t('providerHub.page') }} {{ purchasePage }} · {{ t('providerHub.rowsShown', { shown: purchases.length, total: purchaseData?.total ?? 0 }) }}</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="purchasesLoading || purchasePage <= 1" @click="purchasePage--; loadPurchases()">{{ t('providerHub.previous') }}</button><button class="btn btn-secondary" :disabled="purchasesLoading || purchasePage * 20 >= (purchaseData?.total ?? 0)" @click="purchasePage++; loadPurchases()">{{ t('providerHub.next') }}</button></div></div>
            </section>
            <section v-if="tab === 'rules'" class="space-y-4">
              <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ wallet.name }} · {{ t('providerHub.rules') }}</h3><button class="btn btn-primary" :disabled="busy || !wallet.active" @click="open('rule')"><Icon name="plus" size="sm" />{{ t('providerHub.addRule') }}</button></div>
              <div class="table-scroll"><table class="hub-table"><thead><tr><th>{{ t('providerHub.model') }}</th><th>{{ t('providerHub.method') }}</th><th>{{ t('providerHub.baseRate') }}</th><th>{{ t('providerHub.currency') }}</th><th>{{ t('providerHub.started') }}</th><th>{{ t('providerHub.actions') }}</th></tr></thead><tbody><tr v-for="rule in rules" :key="rule.id"><td class="max-w-64 break-all">{{ rule.model }}<div class="text-xs text-gray-500">#{{ rule.id }}</div></td><td>{{ t(`providerHub.methods.${rule.method}`) }}</td><td>{{ rule.method === 'standard_multiplier' ? rule.multiplier : rule.method === 'account_credits' ? amount(wallet.credit_cost, wallet.currency) : rule.method === 'tokens' ? `${rule.input_price} / ${rule.output_price}` : rule.unit_price }}</td><td>{{ rule.currency }}</td><td class="whitespace-nowrap text-xs">{{ date(rule.started_at) }}</td><td><div class="flex gap-1"><button class="icon-button" :title="t('providerHub.edit')" :aria-label="t('providerHub.edit')" :disabled="busy" @click="open('rule', rule)"><Icon name="edit" size="sm" /></button><button class="btn btn-secondary" :disabled="busy" @click="confirmAction(`/rules/${rule.id}/close`, { revision: wallet.revision }, 'confirmMessage')">{{ t('providerHub.stopRule') }}</button></div></td></tr><tr v-if="!rules.length"><td colspan="6">{{ t('providerHub.ruleMissing') }}</td></tr></tbody></table></div>
            </section>
            <section v-if="tab === 'keys'" class="space-y-4">
              <div class="flex flex-wrap justify-between gap-2"><h3 class="font-semibold">{{ wallet.name }} · {{ t('providerHub.keys') }}</h3><div class="flex gap-2"><button class="btn btn-secondary" :disabled="busy || !wallet.has_secret" @click="loadKeys">{{ t('providerHub.refresh') }}</button><button class="btn btn-primary" :disabled="busy || !wallet.has_secret" @click="openCreateKey"><Icon name="plus" size="sm" />{{ t('providerHub.createKey') }}</button><button class="btn btn-primary" :disabled="busy || !selectedKeys.length" @click="open('import')">{{ t('providerHub.import') }}</button></div></div>
              <div class="table-scroll"><table class="hub-table"><thead><tr><th></th><th>{{ t('providerHub.keyName') }}</th><th>{{ t('providerHub.keyGroup') }}</th><th>{{ t('providerHub.maskedKey') }}</th><th>{{ t('providerHub.available') }}</th></tr></thead><tbody><tr v-for="key in remoteKeys" :key="key.id"><td><input v-model="selectedKeys" type="checkbox" :value="key.id" :disabled="!key.available || busy" :aria-label="key.name" /></td><td>{{ key.name }}</td><td>{{ key.group }}</td><td class="font-mono text-xs">{{ key.masked_key || '—' }}</td><td>{{ key.available ? t('providerHub.available') : t('providerHub.unavailable') }}</td></tr><tr v-if="!remoteKeys.length"><td colspan="5">{{ t('providerHub.empty') }}</td></tr></tbody></table></div>
            </section>
          </template>
        </main>
      </div>
    </div>
    <BaseDialog :show="Boolean(mode)" :title="dialogTitle" width="wide" :close-on-click-outside="!busy" :close-on-escape="!busy" @close="closeEditor">
      <form id="provider-hub-form" class="space-y-4" @submit.prevent="saveEditor">
        <p v-if="editorError" role="alert" class="text-sm text-red-600">{{ editorError }}</p>
        <fieldset :disabled="busy" class="grid gap-4 sm:grid-cols-2">
          <label v-if="mode === 'wallet'" class="field">{{ t('providerHub.syncInterval') }}<select v-model.number="form.sync_interval_minutes" class="input"><option :value="0">{{ t('providerHub.syncOff') }}</option><option v-for="minutes in [5,10,15,30,60,120,360,720,1440]" :key="minutes" :value="minutes">{{ minutes }}</option></select></label>
          <template v-if="mode === 'supplier'">
            <label class="field">{{ t('providerHub.name') }}<input v-model="form.name" class="input" maxlength="120" required /></label><label class="field">{{ t('providerHub.siteType') }}<select v-model="form.site_type" class="input"><option value="sub2api">Sub2API</option><option value="newapi">New API</option><option value="manual">{{ t('providerHub.manual') }}</option></select></label>
            <label class="field sm:col-span-2">{{ t('providerHub.baseUrl') }}<input v-model="form.base_url" class="input" type="url" :required="form.site_type !== 'manual'" /></label><label class="field sm:col-span-2">{{ t('providerHub.notes') }}<textarea v-model="form.notes" class="input" rows="3" maxlength="2000" /></label>
          </template>
          <template v-else-if="mode === 'wallet'">
            <label class="field">{{ t('providerHub.name') }}<input v-model="form.name" class="input" maxlength="120" required /></label><label class="field">{{ t('providerHub.identity') }}<input v-model="form.identity" class="input" maxlength="160" :disabled="editingId > 0" required /></label><label class="field">{{ t('providerHub.currency') }}<input v-model="form.currency" class="input" maxlength="3" pattern="[A-Za-z]{3}" :disabled="editingId > 0" required /></label><label class="field">{{ t('providerHub.creditCurrency') }}<input v-model="form.credit_currency" class="input" maxlength="3" pattern="[A-Za-z]{3}" :disabled="editingId > 0" required /></label><label class="field">{{ t('providerHub.lowBalance') }}<input v-model="form.low_balance" class="input" type="number" min="0" step="any" /></label><p class="text-xs text-gray-500 sm:col-span-2">{{ t('providerHub.walletIdentityNote') }}</p>
          </template>
          <template v-else-if="mode === 'authorization'">
            <p class="text-sm text-gray-500 sm:col-span-2">{{ t('providerHub.connectionScope') }}</p><label class="field">{{ t('providerHub.authMode') }}<select v-model="form.auth_mode" class="input"><option value="none">{{ t('providerHub.none') }}</option><option value="token">{{ t('providerHub.token') }}</option><option v-if="supplier?.site_type === 'sub2api'" value="password">{{ t('providerHub.password') }}</option></select></label><label v-if="form.auth_mode === 'password'" class="field">{{ t('providerHub.username') }}<input v-model="form.username" class="input" autocomplete="off" required /></label><label v-if="supplier?.site_type === 'newapi' && form.auth_mode === 'token'" class="field">{{ t('providerHub.remoteUserId') }}<input v-model="form.remote_user_id" class="input" maxlength="128" /></label><label v-if="form.auth_mode !== 'none'" class="field sm:col-span-2">{{ t('providerHub.secret') }}<input v-model="form.secret" class="input" type="password" autocomplete="new-password" :placeholder="wallet?.has_secret ? t('providerHub.keepSecret') : ''" :required="!wallet?.has_secret" /></label>
          </template>
          <template v-else-if="mode === 'bindings'">
            <input v-model="accountSearch" class="input sm:col-span-2" :placeholder="t('providerHub.search')" :aria-label="t('providerHub.search')" />
            <div class="max-h-80 overflow-y-auto divide-y divide-gray-100 border-y border-gray-200 sm:col-span-2 dark:divide-dark-700 dark:border-dark-700"><label v-for="account in accountChoices" :key="account.id" class="flex gap-3 py-3 text-sm"><input v-model="form.account_ids" class="mt-1" type="checkbox" :value="account.id" /><span class="min-w-0 break-words">#{{ account.id }} {{ account.name }}<span class="block text-xs text-gray-500">{{ account.platform }} · {{ account.type }}<span v-if="accountWallet(account.id)"> · {{ accountWallet(account.id) }}</span></span></span></label></div><label class="flex items-start gap-2 text-sm sm:col-span-2"><input v-model="form.reassign" type="checkbox" />{{ t('providerHub.reassign') }}</label>
          </template>
          <template v-else-if="mode === 'purchase'">
            <label class="field">{{ t('providerHub.kind') }}<select v-model="form.kind" class="input"><option value="topup">{{ t('providerHub.topup') }}</option><option value="refund">{{ t('providerHub.refund') }}</option><option value="opening">{{ t('providerHub.opening') }}</option></select></label><label class="field">{{ t('providerHub.occurredAt') }}<input v-model="form.occurred_at" class="input" type="datetime-local" required /></label><label class="field">{{ t('providerHub.cash') }}<input v-model.number="form.cash" class="input" type="number" min="0" step="any" required /></label><label class="field">{{ t('providerHub.cashCurrency') }}<input v-model="form.currency" class="input" maxlength="3" pattern="[A-Za-z]{3}" required /></label><label class="field sm:col-span-2">{{ t('providerHub.fxRate') }}<input v-model.number="form.fx_rate" class="input" type="number" min="0.000000000001" step="any" required /></label><label class="field">{{ t('providerHub.credits') }}<input v-model.number="form.credits" class="input" type="number" min="0" step="any" required /></label><label class="field">{{ t('providerHub.bonus') }}<input v-model.number="form.bonus" class="input" type="number" min="0" step="any" required /></label><label class="field sm:col-span-2">{{ t('providerHub.reference') }}<input v-model="form.reference" class="input" maxlength="256" /></label><label class="field sm:col-span-2">{{ t('providerHub.note') }}<textarea v-model="form.note" class="input" maxlength="2000" rows="2" /></label>
          </template>
          <template v-else-if="mode === 'rule'">
            <label class="field">{{ t('providerHub.model') }}<input v-model="form.model" class="input" maxlength="160" required /></label><label class="field">{{ t('providerHub.method') }}<select v-model="form.method" class="input"><option v-for="method in methods" :key="method" :value="method">{{ t(`providerHub.methods.${method}`) }}</option></select></label><label class="field">{{ t('providerHub.currency') }}<input v-model="form.currency" class="input" maxlength="3" pattern="[A-Za-z]{3}" required /></label><label class="field">{{ t('providerHub.fxToUsd') }}<input v-model="form.fx_to_usd" class="input" type="number" min="0.000000000001" step="any" :disabled="form.currency.toUpperCase() === 'USD'" /></label>
            <label v-if="form.method === 'standard_multiplier'" class="field">{{ t('providerHub.multiplier') }}<input v-model.number="form.multiplier" class="input" type="number" min="0" step="any" required /></label>
            <template v-if="form.method === 'tokens'"><label v-for="field in tokenFields" :key="field" class="field">{{ t(`providerHub.${fieldLabels[field]}`) }}<input v-model.number="form[field]" class="input" type="number" min="0" step="any" required /></label><label class="field">{{ t('providerHub.cacheWrite12hPrice') }}<input v-model.number="form.cache_write_12h_price" class="input" type="number" min="0" step="any" :placeholder="t('providerHub.cacheWrite12hFallback')" /><span class="text-xs text-gray-500">{{ t('providerHub.cacheWrite12hFallback') }}</span></label></template>
            <label v-if="['image','video_second','video_item','request'].includes(form.method)" class="field">{{ t('providerHub.unitPrice') }}<input v-model.number="form.unit_price" class="input" type="number" min="0" step="any" required /></label><label class="field sm:col-span-2">{{ t('providerHub.note') }}<textarea v-model="form.note" class="input" rows="2" maxlength="2000" /></label><p class="text-xs text-gray-500 sm:col-span-2">{{ t('providerHub.priceNote') }}</p>
          </template>
          <template v-else-if="mode === 'import'">
            <p class="text-sm sm:col-span-2">{{ t('providerHub.selected', { count: selectedKeys.length }) }}</p><label class="field">{{ t('providerHub.platform') }}<select v-model="form.platform" class="input"><option v-for="platform in platforms" :key="platform">{{ platform }}</option></select></label><label class="field">{{ t('providerHub.prefix') }}<input v-model="form.prefix" class="input" maxlength="60" /></label><label class="field sm:col-span-2">{{ t('providerHub.groupIds') }}<input v-model="form.group_ids" class="input" /></label><p class="text-xs text-gray-500 sm:col-span-2">{{ t('providerHub.importGroupNote') }}</p>
          </template>
          <template v-else-if="mode === 'createKey'"><p class="text-sm text-gray-500 sm:col-span-2">{{ t('providerHub.keyCreateWarning') }}</p><label class="field">{{ t('providerHub.keyName') }}<input v-model="form.name" class="input" maxlength="100" required /></label><label class="field">{{ t('providerHub.keyGroup') }}<select v-model="form.group" class="input" required><option v-for="group in remoteGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label></template>
        </fieldset>
      </form>
      <template #footer><div class="flex justify-end gap-2"><button class="btn btn-secondary" :disabled="busy" @click="closeEditor">{{ t('providerHub.cancel') }}</button><button class="btn btn-primary" form="provider-hub-form" :disabled="busy" type="submit"><Icon name="check" size="sm" />{{ busy ? t('providerHub.loading') : t('providerHub.save') }}</button></div></template>
    </BaseDialog>
    <ConfirmDialog :show="Boolean(confirmation)" :title="t('providerHub.confirmTitle')" :message="confirmation ? t(`providerHub.${confirmation.message}`) : ''" danger @cancel="confirmation = null" @confirm="performConfirmation" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { getProviderHub, getProviderPurchases, getProviderKeys, getProviderGroups, providerHubWrite, type ProviderHubState, type Supplier, type SupplierWallet, type SupplierRule, type ProviderKey, type ProviderGroup, type ProviderPurchasePage } from '@/api/admin/providerHub'

const { t } = useI18n()
const state = ref<ProviderHubState | null>(null), loading = ref(false), busy = ref(false), error = ref(''), message = ref(''), editorError = ref('')
const supplierId = ref(0), walletId = ref(0), showArchived = ref(false), tab = ref('accounts'), mode = ref(''), editingId = ref(0), editingRevision = ref(0)
const tabs = ['accounts', 'purchases', 'rules', 'keys'], methods = ['standard_multiplier', 'account_credits', 'tokens', 'image', 'video_second', 'video_item', 'request'], platforms = ['openai', 'anthropic', 'gemini', 'grok', 'deepseek', 'qwen', 'minimax', 'zhipu']
const tokenFields = ['input_price', 'output_price', 'cache_read_price', 'cache_write_price'] as const
const fieldLabels = { input_price: 'inputPrice', output_price: 'outputPrice', cache_read_price: 'cacheReadPrice', cache_write_price: 'cacheWritePrice' }
const remoteKeys = ref<ProviderKey[]>([]), remoteGroups = ref<ProviderGroup[]>([]), selectedKeys = ref<string[]>([]), accountSearch = ref('')
watch(walletId, () => { remoteKeys.value = []; selectedKeys.value = [] })
const confirmation = ref<{ path: string; body: Record<string, unknown>; message: string } | null>(null)
const purchaseData = ref<ProviderPurchasePage | null>(null), purchasePage = ref(1), purchasesLoading = ref(false)
let purchaseSequence = 0
watch([walletId, tab], () => { purchasePage.value = 1; purchaseData.value = null; purchaseSequence++; if (tab.value === 'purchases') void loadPurchases() })
const suppliers = computed(() => state.value?.suppliers.filter(s => showArchived.value || s.active) ?? [])
const supplier = computed(() => state.value?.suppliers.find(s => s.id === supplierId.value))
const wallets = computed(() => state.value?.wallets.filter(w => w.supplier_id === supplierId.value && (showArchived.value || w.active)) ?? [])
const wallet = computed(() => wallets.value.find(w => w.id === walletId.value))
const bindings = computed(() => state.value?.bindings.filter(b => b.wallet_id === walletId.value) ?? [])
const purchases = computed(() => purchaseData.value?.items ?? [])
const rules = computed(() => state.value?.rules.filter(r => r.wallet_id === walletId.value) ?? [])
const accountChoices = computed(() => (state.value?.accounts ?? []).filter(a => !bindings.value.some(b => b.account_id === a.id) && `${a.id} ${a.name}`.toLowerCase().includes(accountSearch.value.toLowerCase())).slice(0, 200))
const dialogTitle = computed(() => t(`providerHub.${({ supplier: editingId.value ? 'editSupplier' : 'newSupplier', wallet: editingId.value ? 'editWallet' : 'newWallet', authorization: 'authorization', bindings: 'bind', purchase: 'purchase', rule: 'addRule', import: 'import', createKey: 'createKey' } as Record<string, string>)[mode.value] || 'title'}`))
function localTime(d = new Date()) { return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16) }
function defaults() { return { name: '', site_type: 'sub2api', base_url: '', notes: '', identity: '', currency: 'USD', credit_currency: 'USD', low_balance: '' as string | number, sync_interval_minutes: 0, auth_mode: 'token', username: '', remote_user_id: '', secret: '', account_ids: [] as number[], reassign: false, kind: 'topup', cash: 0, fx_rate: 1, credits: 0, bonus: 0, occurred_at: localTime(), reference: '', note: '', model: '*', method: 'standard_multiplier', multiplier: 1, input_price: 0, output_price: 0, cache_read_price: 0, cache_write_price: 0, cache_write_12h_price: '' as number | string, unit_price: 0, fx_to_usd: '' as number | string, platform: 'openai', prefix: '', group_ids: '', group: '', operation_id: crypto.randomUUID() } }
const form = reactive(defaults())
let alive = true, sequence = 0, controller: AbortController | undefined
function amount(value: number | null | undefined, unit = '') { return value == null || !Number.isFinite(Number(value)) ? t('providerHub.unknown') : `${Number(value).toLocaleString(undefined, { maximumFractionDigits: 8 })} ${unit}` }
function date(value: string | null) { return value && Number.isFinite(new Date(value).getTime()) ? new Date(value).toLocaleString() : t('providerHub.unknown') }
function accountWallet(id: number) { const b = state.value?.bindings.find(b => b.account_id === id); return b ? state.value?.wallets.find(w => w.id === b.wallet_id)?.name : '' }
function chooseSupplier(id: number) { supplierId.value = id; walletId.value = wallets.value[0]?.id ?? 0; remoteKeys.value = []; selectedKeys.value = [] }
async function load() {
  const request = ++sequence; controller?.abort(); controller = new AbortController(); loading.value = true
  try { const next = await getProviderHub(controller.signal); if (!alive || request !== sequence) return; state.value = next; if (!suppliers.value.some(s => s.id === supplierId.value)) chooseSupplier(suppliers.value[0]?.id ?? 0); if (!wallets.value.some(w => w.id === walletId.value)) walletId.value = wallets.value[0]?.id ?? 0 }
  catch { if (alive && request === sequence) error.value = t('providerHub.failed') }
  finally { if (alive && request === sequence) loading.value = false }
}
function closeEditor() { if (busy.value) return; mode.value = ''; form.secret = ''; editorError.value = '' }
function open(next: string, value?: Supplier | SupplierWallet | SupplierRule) {
  Object.assign(form, defaults()); editingId.value = value?.id ?? 0; editingRevision.value = value && 'revision' in value ? value.revision : 0; editorError.value = ''; accountSearch.value = ''
  if (wallet.value) { form.currency = wallet.value.currency; form.credit_currency = wallet.value.credit_currency }
  if (value) { Object.assign(form, value); if ('low_balance' in value) form.low_balance = value.low_balance ?? ''; if ('fx_to_usd' in value) form.fx_to_usd = value.fx_to_usd ?? ''; if ('cache_write_12h_price' in value) form.cache_write_12h_price = value.cache_write_12h_price ?? ''; if (next === 'authorization' || next === 'wallet') walletId.value = value.id }
  if (next === 'authorization' && form.auth_mode === 'none') form.auth_mode = 'token'
  mode.value = next
}
async function saveEditor() {
  if (busy.value) return
  busy.value = true; editorError.value = ''; error.value = ''; message.value = ''
  try {
    let path = '', method: 'post' | 'put' = 'post'; let body: Record<string, unknown> = {}
    const w = wallet.value
    if (mode.value === 'supplier') { path = `/suppliers${editingId.value ? `/${editingId.value}` : ''}`; method = editingId.value ? 'put' : 'post'; body = { name: form.name, site_type: form.site_type, base_url: form.base_url, notes: form.notes, revision: editingRevision.value } }
    else if (mode.value === 'wallet') { path = `/wallets${editingId.value ? `/${editingId.value}` : ''}`; method = editingId.value ? 'put' : 'post'; body = { name: form.name, identity: form.identity, supplier_id: supplierId.value, currency: form.currency, credit_currency: form.credit_currency, low_balance: form.low_balance === '' ? null : Number(form.low_balance), sync_interval_minutes: form.sync_interval_minutes, revision: editingRevision.value } }
    else if (!w) throw new Error('wallet')
    else if (mode.value === 'authorization') { path = `/wallets/${w.id}/authorization`; method = 'put'; body = { revision: w.revision, auth_mode: form.auth_mode, username: form.username, remote_user_id: form.remote_user_id, secret: form.secret } }
    else if (mode.value === 'bindings') { path = `/wallets/${w.id}/bindings`; method = 'put'; body = { revision: w.revision, account_ids: form.account_ids, reassign: form.reassign } }
    else if (mode.value === 'purchase') { path = `/wallets/${w.id}/purchases`; body = { operation_id: form.operation_id, kind: form.kind, cash: form.cash, currency: form.currency, fx_rate: form.fx_rate, credits: form.credits, bonus: form.bonus, occurred_at: new Date(form.occurred_at).toISOString(), reference: form.reference, note: form.note } }
    else if (mode.value === 'rule') { path = `/wallets/${w.id}/rules`; body = { revision: w.revision, model: form.model, method: form.method, multiplier: form.multiplier, input_price: form.input_price, output_price: form.output_price, cache_read_price: form.cache_read_price, cache_write_price: form.cache_write_price, cache_write_12h_price: form.cache_write_12h_price == null || form.cache_write_12h_price === '' ? null : Number(form.cache_write_12h_price), unit_price: form.unit_price, currency: form.currency, fx_to_usd: form.fx_to_usd === '' ? null : Number(form.fx_to_usd), note: form.note } }
    else if (mode.value === 'import') { const ids = form.group_ids.trim() ? form.group_ids.split(',').map(x => Number(x.trim())) : []; if (ids.some(x => !Number.isSafeInteger(x) || x <= 0)) throw new Error('groups'); path = `/wallets/${w.id}/import`; body = { key_ids: selectedKeys.value, platform: form.platform, group_ids: ids, prefix: form.prefix, revision: w.revision } }
    else if (mode.value === 'createKey') { path = `/wallets/${w.id}/keys`; body = { name: form.name, group: form.group, operation_id: form.operation_id } }
    const result = await providerHubWrite(path, body, method) as { account_ids?: number[] }
    if (!alive) return
    message.value = result?.account_ids ? t('providerHub.imported', { ids: result.account_ids.join(', ') }) : t('providerHub.saved'); mode.value = ''; form.secret = ''; await load(); if (tab.value === 'purchases') await loadPurchases()
  } catch { if (alive) { editorError.value = t('providerHub.failed'); await load() } }
  finally { if (alive) busy.value = false }
}
function confirmAction(path: string, body: Record<string, unknown>, key: string) { if (!busy.value) confirmation.value = { path, body, message: key } }
async function performConfirmation() { const next = confirmation.value; if (!next || busy.value) return; busy.value = true; try { await providerHubWrite(next.path, next.body); confirmation.value = null; message.value = t('providerHub.saved'); await load(); if (tab.value === 'purchases') await loadPurchases() } catch { error.value = t('providerHub.failed') } finally { busy.value = false } }
async function loadPurchases() { const id = walletId.value, seq = ++purchaseSequence; if (!id) return; purchasesLoading.value = true; try { const next = await getProviderPurchases(id, purchasePage.value); if (alive && seq === purchaseSequence && id === walletId.value) purchaseData.value = next } catch { if (alive && seq === purchaseSequence) error.value = t('providerHub.failed') } finally { if (alive && seq === purchaseSequence) purchasesLoading.value = false } }
async function syncWallet(w: SupplierWallet) { busy.value = true; error.value = ''; try { await providerHubWrite(`/wallets/${w.id}/sync`, {}); message.value = t('providerHub.synced') } catch { error.value = t('providerHub.failed') } finally { await load(); busy.value = false } }
async function loadKeys() { if (!wallet.value || busy.value) return; const id = wallet.value.id; busy.value = true; error.value = ''; selectedKeys.value = []; try { const keys = await getProviderKeys(id); if (alive && walletId.value === id) remoteKeys.value = keys } catch { if (alive) error.value = t('providerHub.failed') } finally { if (alive) busy.value = false } }
async function openCreateKey() { if (!wallet.value || busy.value) return; busy.value = true; try { remoteGroups.value = await getProviderGroups(wallet.value.id); open('createKey'); form.group = remoteGroups.value[0]?.id ?? '' } catch { error.value = t('providerHub.failed') } finally { busy.value = false } }
onMounted(load)
onUnmounted(() => { alive = false; sequence++; controller?.abort(); form.secret = '' })
</script>

<style scoped>
.field { @apply flex min-w-0 flex-col gap-1.5 text-sm; }
.icon-button { @apply inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-gray-200 text-gray-500 hover:bg-gray-100 disabled:opacity-40 dark:border-dark-700 dark:hover:bg-dark-800; }
.table-scroll { @apply min-w-0 overflow-x-auto border-y border-gray-200 dark:border-dark-700; }
.hub-table { @apply w-full text-left text-sm; }
.hub-table:has(th:nth-child(4)) { min-width: 600px; }
.hub-table:has(th:nth-child(6)) { min-width: 760px; }
.hub-table th { @apply whitespace-nowrap bg-gray-50 px-3 py-3 text-xs font-medium text-gray-500 dark:bg-dark-900; }
.hub-table td { @apply border-t border-gray-100 px-3 py-3 align-top dark:border-dark-700; }
</style>
