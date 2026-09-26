<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import { useAuthStore } from '@/stores/auth'
import { platformLabel } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'
import { groupTextRate, visibleCatalogGroups } from './modelCatalog'
import { priceTableRows, productFor, type PriceTableRow } from './catalogTable'

const props = defineProps<{ response: ModelPlazaResponse | null; loading: boolean; error?: boolean; embedded?: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()
const authStore = useAuthStore()
const { isDarkMode } = useDocumentDarkMode()
const { copyToClipboard } = useClipboard()
const selectedProduct = ref('')
const selectedGroupId = ref<number | null>(null)
const priceMode = ref<'group' | 'official'>('group')
const groups = computed(() => visibleCatalogGroups(props.response?.groups ?? []))
const productOrder = ['anthropic', 'openai', 'grok', 'gemini', 'antigravity', 'deepseek', 'image', 'video']
const products = computed(() => [...new Set(groups.value.flatMap(group => group.models.map(productFor)))].sort((a, b) => {
  const rank = (p: string) => productOrder.includes(p) ? productOrder.indexOf(p) : 99
  return rank(a) - rank(b)
}))
const productGroups = computed(() => groups.value.filter(group => group.models.some(model => productFor(model) === selectedProduct.value)))
const selectedGroup = computed(() => productGroups.value.find(group => group.id === selectedGroupId.value))
const rows = computed(() => selectedGroup.value ? priceTableRows(selectedGroup.value, selectedProduct.value) : [])
const tokenTable = computed(() => rows.value.some(row => row.kind === 'token'))
const columns = computed(() => [
  { key: 'input', label: t('modelPlaza.reference.input') },
  { key: 'output', label: t('modelPlaza.reference.output') },
  { key: 'cache_write', label: t('modelPlaza.reference.cacheWrite') },
  { key: 'cache_read', label: t('modelPlaza.reference.cacheRead') },

])
const descriptionHtml = computed(() => props.response?.description?.trim()
  ? DOMPurify.sanitize(marked.parse(props.response.description) as string) : '')
watch(products, next => { if (!next.includes(selectedProduct.value)) selectedProduct.value = next[0] ?? '' }, { immediate: true })
watch(productGroups, next => { if (!next.some(group => group.id === selectedGroupId.value)) selectedGroupId.value = next[0]?.id ?? null }, { immediate: true })
function productLabel(product: string) {
  return ({ anthropic: 'Claude', openai: 'ChatGPT', grok: 'Grok', gemini: 'Gemini', deepseek: 'DeepSeek', image: t('modelPlaza.catalog.kinds.image'), video: t('modelPlaza.catalog.kinds.video') } as Record<string, string>)[product] ?? platformLabel(product)
}
function productIcon(product: string) {
  return ({ anthropic: 'claude', openai: 'gpt', image: 'gpt-image', video: 'sora' } as Record<string, string>)[product] ?? product
}
function groupRate(group: ModelPlazaGroup) {
  if (selectedProduct.value === 'video' && group.platform === 'video') return t('videoModule.finalPrice')
  const rate = selectedProduct.value === 'image' && group.image_rate_independent ? group.image_rate_multiplier
    : selectedProduct.value === 'video' && group.video_rate_independent ? group.video_rate_multiplier : groupTextRate(group)
  return `×${Number(rate.toFixed(4))}`
}
function metric(row: PriceTableRow, key: string) { return row.metrics.find(item => item.key === key) }
function price(row: PriceTableRow, key: string, official = priceMode.value === 'official') {
  const item = metric(row, key)
  return formatScaled((official ? item?.official : item?.current) ?? null, row.kind === 'token' ? 1_000_000 : 1, 2)
}
function saving(row: PriceTableRow) { return row.metrics.find(item => item.key === 'input')?.saving ?? null }
function unit(row: PriceTableRow) {
  return row.kind === 'token' ? '1M tokens' : t(`modelPlaza.catalog.units.${row.kind === 'image' ? 'image' : row.kind === 'video' ? 'second' : 'request'}`)
}
function copyModelId(modelId: string) { void copyToClipboard(modelId, t('modelPlaza.catalog.copySuccess')) }
</script>

<template>
  <main class="model-catalog" :class="{ 'is-dark': isDarkMode }">
    <nav v-if="products.length" class="product-tabs" :aria-label="t('modelPlaza.reference.products')">
      <button v-for="product in products" :key="product" type="button" :aria-pressed="selectedProduct === product" :class="{ active: selectedProduct === product }" @click="selectedProduct = product">
        <ModelIcon :model="productIcon(product)" size="22px" /><span>{{ productLabel(product) }}</span>
      </button>
    </nav>
    <div class="pricing-rule-bar"><Icon name="calculator" size="sm" /><strong>{{ t('nav.billingRules') }}</strong><span>{{ t('modelPlaza.reference.rule') }}</span></div>
    <section class="pricing-list-panel" :aria-label="t('modelPlaza.reference.title')">
      <header class="pricing-list-heading">
        <h2><Icon name="shield" size="md" />{{ t('modelPlaza.reference.title') }}</h2>
        <p>{{ t('modelPlaza.reference.subtitle') }}</p>
        <div class="price-mode-switch" role="group" :aria-label="t('modelPlaza.reference.mode')">
          <button type="button" :aria-pressed="priceMode === 'group'" @click="priceMode = 'group'">{{ t('modelPlaza.reference.groupPrice') }}</button>
          <button type="button" :aria-pressed="priceMode === 'official'" @click="priceMode = 'official'">{{ t('modelPlaza.catalog.officialPrice') }}</button>
        </div>
      </header>
      <div class="pricing-list-body">
        <div v-if="productGroups.length" class="pricing-group-scroll" :aria-label="t('modelPlaza.catalog.groupSelector')">
          <button v-for="group in productGroups" :key="group.id" type="button" class="pricing-group-card" :class="{ active: selectedGroupId === group.id }" :aria-pressed="selectedGroupId === group.id" @click="selectedGroupId = group.id">
            <span class="pricing-group-title"><strong :title="group.name">{{ group.name }}</strong><Icon v-if="selectedGroupId === group.id" name="checkCircle" size="sm" /></span>
            <span class="pricing-group-meta"><small :title="group.user_rate_multiplier != null ? t('modelPlaza.catalog.personalRate') : undefined">{{ groupRate(group) }}</small><span>{{ t(group.user_rate_multiplier != null ? 'modelPlaza.catalog.personalRate' : 'modelPlaza.catalog.effectiveMultiplier') }}</span></span>
          </button>
        </div>
        <div v-if="selectedGroup" class="group-description"><strong>{{ t('modelPlaza.reference.groupIntro') }}</strong><span>{{ selectedGroup.description || t('modelPlaza.detail.defaultDescription') }}</span></div>
        <div v-if="loading" class="state-panel" role="status"><div class="state-spinner"></div>{{ t('modelPlaza.loading') }}</div>
        <div v-else-if="error" class="state-panel"><p>{{ t('modelPlaza.loadFailed') }}</p><button type="button" class="retry-button" @click="emit('retry')">{{ t('common.refresh') }}</button></div>
        <div v-else-if="rows.length === 0" class="state-panel">{{ t('modelPlaza.empty') }}</div>
        <template v-else>
          <div class="pricing-table-scroll" tabindex="0" :aria-label="t('modelPlaza.reference.title')">
            <table class="reference-pricing-table">
              <thead><tr><th scope="col">{{ t('modelPlaza.catalog.modelId') }}</th><template v-if="tokenTable"><th v-for="column in columns" :key="column.key" scope="col">{{ column.label }}</th><th scope="col">{{ t('modelPlaza.table.savingsColumn') }}</th></template><template v-else><th scope="col">{{ t('modelPlaza.table.tier') }}</th><th scope="col">{{ t('modelPlaza.catalog.currentPrice') }}</th></template></tr></thead>
              <tbody>
                <tr v-for="row in rows" :key="row.key" :data-billing-kind="row.kind">
                  <th scope="row" class="model-id-cell"><div><code>{{ row.modelId }}</code><button type="button" class="copy-id-button" :aria-label="`${t('modelPlaza.catalog.copyModelId')}: ${row.modelId}`" @click="copyModelId(row.modelId)"><Icon name="copy" size="xs" /></button></div>
                    <small class="model-platform">{{ platformLabel(row.model.platform || selectedGroup?.platform || '') }}</small>
                    <small v-if="tokenTable && row.tier">{{ row.tier }}</small><small v-if="row.period" class="period-label">{{ row.period }}</small>
                    <small v-if="row.model.long_context_basis === 'marginal'">{{ t('modelPlaza.reference.marginal') }}</small>
                    <small v-if="row.model.pricing?.reasoning_effort_multipliers && Object.keys(row.model.pricing.reasoning_effort_multipliers).length" class="effort-note"><template v-for="(rate, effort) in row.model.pricing.reasoning_effort_multipliers" :key="effort">{{ effort }} ×{{ rate }} · </template></small>
                  </th>
                  <template v-if="row.kind === 'token'"><td v-for="column in columns" :key="column.key">
                    <div class="price-primary"><strong>{{ price(row, column.key) }}</strong><small v-if="price(row, column.key) !== '-'"> / {{ unit(row) }}</small></div>
                    <div v-if="priceMode === 'group' && metric(row, column.key)?.official != null" class="price-reference" :class="{ discounted: (metric(row, column.key)?.saving ?? 0) > 0 }">{{ t('modelPlaza.catalog.officialPrice') }} {{ price(row, column.key, true) }}</div>
                    <div v-if="column.key === 'cache_write' && metric(row, 'cache_write_1h')?.current != null && (metric(row, 'cache_write_1h')?.current !== metric(row, 'cache_write')?.current || metric(row, 'cache_write_1h')?.official !== metric(row, 'cache_write')?.official)" class="price-cache-hour">1h: {{ price(row, 'cache_write_1h') }}<span v-if="priceMode === 'group' && metric(row, 'cache_write_1h')?.official != null" class="price-reference">{{ t('modelPlaza.catalog.officialPrice') }} {{ price(row, 'cache_write_1h', true) }}</span></div>
                  </td><td><span v-if="priceMode === 'group' && saving(row) != null" class="saving-badge">{{ t('modelPlaza.table.savings', { percent: saving(row) }) }}</span><span v-else class="empty-price">—</span></td></template>
                  <template v-else>
                    <td v-if="!tokenTable">{{ row.tier || '—' }}</td>
                    <td :colspan="tokenTable ? columns.length + 1 : 1">
                      <div v-if="tokenTable" class="unit-billing-kind">{{ t(`modelPlaza.catalog.kinds.${row.kind}`) }}</div>
                      <div class="price-primary"><strong>{{ price(row, row.metrics[0]!.key) }}</strong><small v-if="priceMode === 'group'"> / {{ unit(row) }}</small></div>
                      <small v-if="priceMode === 'official'" class="empty-price">{{ t('modelPlaza.reference.noOfficial') }}</small>
                    </td>
                  </template>
                </tr>
              </tbody>
            </table>
          </div>
          <p class="catalog-footnote">{{ t('modelPlaza.reference.note') }}</p>
          <p v-if="selectedGroup?.peak_rate_enabled && selectedGroup.platform !== 'video'" class="catalog-footnote">{{ t('modelPlaza.detail.peakNote', { window: `${selectedGroup.peak_start}–${selectedGroup.peak_end}`, multiplier: selectedGroup.peak_rate_multiplier }) }}</p>
        </template>
        <div v-if="descriptionHtml" class="catalog-notice" v-html="descriptionHtml"></div>
        <p v-if="!authStore.isAuthenticated" class="catalog-footnote">{{ t('modelPlaza.anonymousHint') }}</p>
      </div>
    </section>
  </main>
</template>

<style scoped>
.model-catalog { --catalog-surface: var(--kc-panel,#fff); --catalog-muted-surface: var(--kc-panel-muted,#f8faf9); --catalog-border: var(--kc-line,#e5e7eb); --catalog-text: var(--kc-ink,#182b29); --catalog-muted: var(--kc-muted,#7b8382); --catalog-accent: var(--kc-accent-strong,#0d766e); --catalog-tint: var(--kc-accent-soft,#edf7f4); width: 100%; max-width: none; margin: 0; color: var(--catalog-text); }
.model-catalog.is-dark { --catalog-surface: var(--kc-panel,#192523); --catalog-muted-surface: var(--kc-panel-muted,#14211f); --catalog-border: var(--kc-line,#30413d); --catalog-text: var(--kc-ink,#e6eeec); --catalog-muted: var(--kc-muted,#9eaead); --catalog-accent: var(--kc-accent-strong,#6dcdbc); --catalog-tint: var(--kc-accent-soft,#163a33); }
.product-tabs { display: flex; flex-wrap: wrap; gap: 8px; padding: 8px; border: 1px solid var(--catalog-border); border-radius: 8px; background: var(--catalog-surface); box-shadow: 0 1px 3px rgb(0 0 0 / 4%); }
.product-tabs button { display: flex; align-items: center; gap: 8px; min-height: 50px; padding: 10px 16px; border: 1px solid transparent; border-radius: 5px; font-size: 18px; color: var(--catalog-muted); }
.is-dark .product-tabs :deep(path[fill="#000000"]) { fill: var(--catalog-text); }
.product-tabs button.active { color: var(--catalog-accent); border-color: var(--catalog-accent); background: var(--catalog-muted-surface); box-shadow: 0 0 0 3px var(--catalog-tint); }
.pricing-rule-bar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-top: 16px; padding: 13px 20px; border: 1px solid var(--catalog-border); border-radius: 8px; background: var(--catalog-surface); font-size: 12px; color: var(--catalog-muted); }
.pricing-rule-bar strong { color: var(--catalog-text); white-space: nowrap; }
.pricing-rule-bar > svg { color: var(--catalog-accent); }
.pricing-list-panel { margin-top: 16px; border: 1px solid var(--catalog-border); border-radius: 8px; overflow: hidden; background: var(--catalog-surface); box-shadow: 0 1px 3px rgb(0 0 0 / 3%); }
.pricing-list-heading { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; min-height: 66px; padding: 16px 20px; border-bottom: 1px solid var(--catalog-border); }
.pricing-list-heading h2 { display: flex; align-items: center; gap: 10px; font-size: 20px; font-weight: 700; margin-right: auto; }
.pricing-list-heading p { font-size: 12px; color: var(--catalog-muted); }
.price-mode-switch { display: flex; flex: 0 0 auto; border-radius: 99px; padding: 4px; background: var(--catalog-muted-surface); }
.price-mode-switch button { padding: 5px 13px; border-radius: 99px; font-size: 12px; font-weight: 600; }
.price-mode-switch button[aria-pressed='true'] { background: var(--catalog-accent); color: var(--catalog-surface); box-shadow: 0 1px 3px rgb(0 0 0 / 10%); }
.pricing-list-body { padding: 20px; }
.pricing-group-scroll { display: flex; gap: 12px; overflow-x: auto; padding: 0 1px 10px; scrollbar-width: thin; scrollbar-color: #b7c0bd var(--catalog-muted-surface); }
.pricing-group-card { flex: 0 0 auto; width: 270px; padding: 17px 16px; text-align: left; border: 1px solid var(--catalog-border); border-radius: 6px; background: var(--catalog-surface); }
.pricing-group-card.active { border-color: var(--catalog-accent); background: var(--catalog-muted-surface); box-shadow: 0 0 0 2px var(--catalog-tint); }
.pricing-group-title { display: flex; align-items: center; gap: 8px; justify-content: space-between; }
.pricing-group-title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 15px; line-height: 1.5; }
.pricing-group-title > svg { flex-shrink: 0; color: var(--catalog-accent); }
.pricing-group-meta { display: flex; align-items: center; gap: 7px; margin-top: 6px; font-size: 12px; color: var(--catalog-muted); }
.pricing-group-meta small { font-size: 12px; font-variant-numeric: tabular-nums; }
.group-description { display: flex; gap: 14px; margin: 12px 0 16px; padding: 12px 16px; border: 1px solid var(--catalog-border); border-radius: 6px; background: color-mix(in srgb, var(--catalog-tint) 38%,var(--catalog-surface)); font-size: 13px; line-height: 1.5; }
.group-description strong { flex-shrink: 0; color: var(--catalog-accent); }
.group-description span { color: var(--catalog-muted); }
.pricing-table-scroll { overflow-x: auto; border: 1px solid var(--catalog-border); border-radius: 7px; }
.reference-pricing-table { width: 100%; min-width: 890px; border-collapse: collapse; font-size: 13px; text-align: left; }
.reference-pricing-table th,.reference-pricing-table td { padding: 17px 16px; border-bottom: 1px solid var(--catalog-border); vertical-align: middle; }
.reference-pricing-table thead th { background: var(--catalog-muted-surface); font-weight: 500; color: var(--catalog-muted); padding-block: 14px; white-space: nowrap; }
.reference-pricing-table tr:last-child > * { border-bottom: 0; }
.reference-pricing-table tbody tr:hover { background: var(--catalog-muted-surface); }
.model-id-cell { font-weight: 500; min-width: 220px; max-width: 320px; }
.model-id-cell > div { display: flex; align-items: center; gap: 10px; }
.model-id-cell code { font-family: inherit; overflow-wrap: anywhere; }
.copy-id-button { flex-shrink: 0; padding: 4px; color: var(--catalog-muted); border-radius: 4px; }
.copy-id-button:hover { color: var(--catalog-accent); background: var(--catalog-tint); }
.model-id-cell > small { display: block; margin-top: 6px; color: var(--catalog-muted); font-size: 10px; line-height: 1.6; }
.model-id-cell > .period-label { color: var(--catalog-accent); }
.price-primary { white-space: nowrap; }
.price-primary strong { color: var(--catalog-accent); font-size: 15px; font-weight: 700; font-variant-numeric: tabular-nums; }
.price-primary small { font-size: 10px; color: var(--catalog-muted); }
.unit-billing-kind { margin-bottom: 4px; color: var(--catalog-muted); font-size: 11px; }
.price-reference { margin-top: 4px; font-size: 10px; color: var(--catalog-muted); white-space: nowrap; }
.price-cache-hour { margin-top: 6px; font-size: 10px; color: var(--catalog-muted); }
.price-cache-hour .price-reference { display: block; }
.price-reference.discounted { text-decoration: line-through; opacity: .8; }
.saving-badge { display: inline-block; padding: 4px 9px; border-radius: 99px; color: #19844e; background: #effbf3; font-size: 12px; font-weight: 700; white-space: nowrap; }
.is-dark .saving-badge { color: #82d6a5; background: #1c3a2b; }
.catalog-footnote,.catalog-notice { color: var(--catalog-muted); font-size: 12px; line-height: 1.7; margin-top: 14px; }
.catalog-notice :deep(a) { text-decoration: underline; }
.empty-price { font-size: 12px; color: var(--catalog-muted); }
.state-panel { min-height: 220px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; color: var(--catalog-muted); }
.retry-button { padding: 8px 20px; border-radius: 6px; color: var(--catalog-accent); background: var(--catalog-tint); }
.state-spinner { height: 28px; width: 28px; border: 3px solid var(--catalog-border); border-top-color: var(--catalog-accent); border-radius: 50%; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media(max-width: 767px) { .product-tabs { gap: 4px; } .product-tabs button { min-height: 42px; font-size: 14px; padding: 8px 10px; } .pricing-list-heading,.pricing-list-body { padding: 16px; } .pricing-list-heading p { display: none; } .pricing-list-heading h2 { font-size: 18px; } .pricing-group-card { width: 245px; } .group-description { flex-direction: column; gap: 5px; } .pricing-rule-bar { padding: 12px 16px; } }
</style>
