<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PricingFormEntry } from './types'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  isAvailableVideoSupplier, newVideoGroupPricing, videoCustomerPrices,
  videoManualPricePatch, videoPriceSource, videoPricingFromCatalog,
} from './videoGroupPricing'
import { videoUpstreamAPI, type VideoCatalog, type VideoMarkup, type VideoModelConfig, type VideoRuntimeSettings } from '@/api/admin/videoUpstream'

const props = defineProps<{ modelValue: PricingFormEntry[] }>()
const emit = defineEmits<{ 'update:modelValue': [entries: PricingFormEntry[]] }>()
const { t } = useI18n()
const settings = ref<VideoRuntimeSettings>()
const catalog = ref<VideoCatalog>()
const loading = ref(false)
const probing = ref(false)
const error = ref('')
const errorDetail = ref('')
const notice = ref('')
const search = ref('')
const selected = ref<string[]>([])
const zeroMarkup: VideoMarkup = { output_per_second: 0, reference_per_second: 0, per_request: 0 }
const markupFields = ['output_per_second', 'reference_per_second', 'per_request'] as const
const usedModels = computed(() => new Set(props.modelValue.flatMap(entry => entry.models.map(model => model.trim()))))
const catalogRows = computed(() => Object.values(catalog.value?.models || {})
  .filter(price => price.model.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase()))
  .sort((a, b) => a.model.localeCompare(b.model))
  .map(price => {
    const entry = videoPricingFromCatalog(price)
    return { price, available: !!entry, customer: entry ? videoCustomerPrices(entry, settings.value) : null }
  }))
const canAddSelected = computed(() => selected.value.some(model => {
  const price = catalog.value?.models[model]
  return price && isAvailableVideoSupplier(price) && !usedModels.value.has(model)
}))
const rows = computed(() => props.modelValue.map(entry => {
  const catalogCost = catalog.value?.models[entry.models[0]?.trim()]
  const cost = catalogCost ?? entry.video?.cost
  // An unsaved upstream price follows the latest available catalog. Existing
  // customer prices remain cached until save; browser snapshots are not authority.
  const previewCost = catalog.value ? catalogCost ?? null : cost
  const availableCost = previewCost && isAvailableVideoSupplier(previewCost) ? previewCost : null
  const customer = videoCustomerPrices(entry, settings.value, availableCost)
  return {
    entry, customer, cost, availableCost,
    upstream: videoPriceSource(entry.video) === 'upstream',
    costAt: catalogCost ? catalog.value?.last_success : entry.video?.synced_at,
    costFromCatalog: !!catalogCost,
    markup: entry.video?.markup ?? settings.value?.markup ?? zeroMarkup,
    resolutions: [...new Set([...(entry.video?.spec.resolutions || []), ...Object.keys(customer.resolutions)])],
  }
}))

async function load(preserveCatalog = false) {
  loading.value = true
  error.value = ''
  errorDetail.value = ''
  try {
    const data = await videoUpstreamAPI.get()
    settings.value = data.settings
    if (!preserveCatalog) catalog.value = data.catalog
  } catch (cause) {
    error.value = 'videoModule.catalogLoadFailed'
    errorDetail.value = extractApiErrorMessage(cause, '')
  } finally {
    loading.value = false
  }
}
async function probe() {
  probing.value = true
  error.value = ''
  errorDetail.value = ''
  notice.value = ''
  try {
    // Probe only refreshes the public catalog; it must never reprice saved groups.
    catalog.value = await videoUpstreamAPI.probe()
    if (!catalog.value.last_error) notice.value = 'videoModule.probeSuccess'
    if (!settings.value) await load(true)
  } catch (cause) {
    error.value = 'videoModule.probeFailed'
    errorDetail.value = extractApiErrorMessage(cause, '')
    try {
      // The failed probe response may omit diagnostics persisted in the catalog.
      // Read only failure metadata, preserving the displayed costs and their date.
      const data = await videoUpstreamAPI.get()
      if (catalog.value) catalog.value = {
        ...catalog.value, last_attempt: data.catalog.last_attempt, last_error: data.catalog.last_error,
      }
    } catch { /* Keep the original probe error and all editor state. */ }
  } finally {
    probing.value = false
  }
}
onMounted(() => load())

function addSelected() {
  const entries: PricingFormEntry[] = []
  for (const model of selected.value) {
    const price = catalog.value?.models[model]
    if (!price || usedModels.value.has(model)) continue
    const entry = videoPricingFromCatalog(price)
    if (entry) entries.push(entry)
  }
  if (entries.length) emit('update:modelValue', [...props.modelValue, ...entries])
  selected.value = []
}
function update(index: number, patch: Partial<PricingFormEntry>): void {
  emit('update:modelValue', props.modelValue.map((entry, i) => i === index ? { ...entry, ...patch, intervals: [] } : entry))
}
function updateVideo(index: number, patch: Partial<VideoModelConfig>) {
  const video = props.modelValue[index]?.video
  if (video) update(index, { video: { ...video, price_source: videoPriceSource(video), ...patch } })
}
function overrideMarkup(index: number, enabled: boolean) {
  updateVideo(index, { markup: enabled ? { ...(settings.value?.markup ?? zeroMarkup) } : undefined })
}
function setMarkup(index: number, field: keyof VideoMarkup, event: Event) {
  const video = props.modelValue[index]?.video
  if (video?.markup) updateVideo(index, { markup: { ...video.markup, [field]: inputNumber(event) } })
}
function inputNumber(event: Event): number {
  const value = (event.target as HTMLInputElement).value
  return value.trim() === '' ? NaN : Number(value)
}
function followValue(event: Event): boolean | undefined {
  const value = (event.target as HTMLSelectElement).value
  return value === '' ? undefined : value === 'true'
}
function state(value?: boolean): string {
  return t(value == null ? 'videoModule.unknown' : value ? 'videoModule.enabled' : 'videoModule.disabled')
}
function money(value: number | null | undefined): string {
  return value == null || !Number.isFinite(value) ? '—' : String(Number(value.toFixed(8)))
}
function unit(mode: string): string {
  return t(mode === 'per_request' ? 'videoModule.videoUnit' : 'videoModule.secondUnit')
}
function date(value?: string): string {
  if (!value) return t('videoModule.neverSynced')
  const at = new Date(value)
  return Number.isNaN(at.getTime()) ? value : at.toLocaleString()
}
</script>

<template>
  <section class="mt-4 space-y-4 border-t border-gray-200 pt-4 dark:border-dark-400" data-testid="video-group-pricing">
    <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('videoModule.pricingTitle') }}</h4>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('videoModule.pricingHint') }}</p>

    <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-500" data-testid="video-catalog">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h5 class="text-sm font-medium">{{ t('videoModule.catalogTitle') }}</h5>
        <button type="button" class="btn btn-secondary" :disabled="loading || probing" data-testid="video-pricing-probe" @click="probe">{{ t(probing ? 'videoModule.probing' : 'videoModule.probePrices') }}</button>
      </div>
      <p class="text-xs text-gray-500">{{ t('videoModule.catalogHint') }}</p>
      <p class="text-xs text-gray-500" data-testid="video-global-sync">{{ t('videoModule.globalAutoSync') }}: {{ state(settings?.auto_sync) }} · {{ t('videoModule.lastCatalogRead') }}: {{ date(catalog?.last_success) }}</p>
      <p class="text-xs text-gray-500">{{ t('videoModule.lastCatalogAttempt') }}: {{ date(catalog?.last_attempt) }} · {{ t('videoModule.catalogSnapshotHint') }}</p>
      <p v-if="settings" class="text-xs text-gray-500">{{ t('videoModule.globalRules', { output: money(settings.markup.output_per_second), reference: money(settings.markup.reference_per_second), request: money(settings.markup.per_request), increase: state(settings.follow_increase), decrease: state(settings.follow_decrease) }) }}</p>
      <p v-if="catalog?.last_error" class="text-xs text-amber-700 dark:text-amber-400">{{ t('videoModule.catalogLastError') }}: {{ catalog.last_error }}</p>
      <div v-if="error" role="alert" class="text-xs text-red-600 dark:text-red-400">
        {{ t(error) }}
        <p v-if="errorDetail" class="mt-1 whitespace-pre-wrap" data-testid="video-pricing-error-detail">{{ errorDetail }}</p>
        <button v-if="!settings" type="button" class="ml-2 underline" :disabled="loading || probing" @click="load()">{{ t('videoModule.retryLoad') }}</button>
      </div>
      <p v-if="notice" role="status" class="text-xs text-green-700 dark:text-green-400">{{ t(notice) }}</p>
      <p v-if="loading" role="status" class="text-xs text-gray-500">{{ t('videoModule.catalogLoading') }}</p>
      <template v-else>
        <div class="flex flex-wrap gap-2">
          <input v-model="search" type="search" class="input min-w-0 flex-1" :aria-label="t('videoModule.searchCatalog')" :placeholder="t('videoModule.searchCatalog')">
          <button type="button" class="btn btn-secondary" :disabled="!canAddSelected || !settings" data-testid="video-catalog-add" @click="addSelected">{{ t('videoModule.addSelected') }}</button>
        </div>
        <div class="max-h-72 overflow-auto">
          <table v-if="catalogRows.length" class="w-full text-left text-xs">
            <thead><tr><th class="p-2">{{ t('videoModule.model') }}</th><th class="p-2">{{ t('videoModule.upstreamCost') }}</th><th class="p-2">{{ t('videoModule.customerPreview') }}</th></tr></thead>
            <tbody>
              <tr v-for="row in catalogRows" :key="row.price.model" class="border-t border-gray-100 dark:border-dark-500" data-testid="video-catalog-row">
                <td class="max-w-xs p-2 align-top">
                  <label class="flex items-start gap-2 break-all"><input v-model="selected" type="checkbox" :value="row.price.model" :disabled="!row.available || usedModels.has(row.price.model) || !settings" data-testid="video-catalog-select">{{ row.price.model }}</label>
                  <span v-if="usedModels.has(row.price.model)" class="text-gray-500">{{ t('videoModule.alreadyAdded') }}</span>
                  <span v-else-if="!row.available" class="text-amber-700 dark:text-amber-400">{{ row.price.unavailable_reason || row.price.spec?.reason || t('videoModule.specUnavailable') }}</span>
                </td>
                <td class="p-2 align-top" data-testid="video-catalog-cost">
                  <p>{{ t('videoModule.generationPrice') }}: {{ money(row.price.unit_price) }} / {{ unit(row.price.billing_mode) }}</p>
                  <p>{{ t('videoModule.referencePrice') }}: {{ money(row.price.reference_per_second) }} / {{ unit('video') }}</p>
                  <p v-for="(price, resolution) in row.price.resolution_prices" :key="resolution">{{ resolution }}: {{ money(price) }} / {{ unit(row.price.billing_mode) }}</p>
                </td>
                <td class="p-2 align-top" data-testid="video-catalog-customer">
                  <template v-if="row.customer && settings">
                    <p>{{ t('videoModule.generationPrice') }}: {{ money(row.customer.unit) }} / {{ unit(row.price.billing_mode) }}</p>
                    <p>{{ t('videoModule.referencePrice') }}: {{ money(row.customer.reference) }} / {{ unit('video') }}</p>
                    <p v-for="(price, resolution) in row.customer.resolutions" :key="resolution">{{ resolution }}: {{ money(price) }} / {{ unit(row.price.billing_mode) }}</p>
                  </template>
                  <span v-else>—</span>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-else class="py-4 text-center text-xs text-gray-500">{{ t('videoModule.catalogEmpty') }}</p>
        </div>
      </template>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="text-xs text-gray-500">{{ t('videoModule.emptyPricing') }}</p>
      <button type="button" class="btn btn-secondary" data-testid="video-pricing-add" @click="emit('update:modelValue', [...modelValue, newVideoGroupPricing()])">{{ t('videoModule.addModel') }}</button>
    </div>
    <div v-for="(row, index) in rows" :key="index" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-500" data-testid="video-pricing-row">
      <div class="grid grid-cols-1 items-end gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(6rem,1fr)_minmax(6rem,1fr)_auto]">
        <label class="min-w-0 text-xs text-gray-600 dark:text-gray-300">{{ t('videoModule.model') }}
          <input :value="row.entry.models.join(', ')" :readonly="!!row.entry.video" class="input mt-1" required :placeholder="t('videoModule.modelPlaceholder')" data-testid="video-pricing-model" @input="update(index, { models: [($event.target as HTMLInputElement).value] })">
        </label>
        <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('videoModule.billing') }}
          <select :value="row.entry.billing_mode" :disabled="!!row.entry.video" class="input mt-1" data-testid="video-pricing-billing" @change="update(index, { billing_mode: ($event.target as HTMLSelectElement).value as 'video' | 'per_request' })">
            <option value="per_request">{{ t('videoModule.perVideo') }}</option><option value="video">{{ t('videoModule.perSecond') }}</option>
          </select>
        </label>
        <label class="text-xs text-gray-600 dark:text-gray-300">{{ t(row.entry.video ? 'videoModule.manualPrice' : 'videoModule.price') }} ({{ t('videoModule.accountingUnit') }} / {{ unit(row.entry.billing_mode) }})
          <input :value="row.upstream ? '' : row.entry.per_request_price" type="number" min="0" step="any" :required="!row.upstream" class="input mt-1" :placeholder="row.entry.video ? t('videoModule.upstreamPlaceholder') : ''" data-testid="video-pricing-price" @input="update(index, videoManualPricePatch(row.entry, ($event.target as HTMLInputElement).value, settings, row.availableCost))">
        </label>
        <button type="button" class="btn btn-secondary" :aria-label="t('videoModule.removeModel')" data-testid="video-pricing-remove" @click="emit('update:modelValue', modelValue.filter((_, i) => i !== index))">{{ t('common.delete') }}</button>
      </div>
      <p class="text-xs text-gray-500" data-testid="video-price-source">{{ t(row.entry.video ? row.upstream ? 'videoModule.upstreamMode' : 'videoModule.manualMode' : 'videoModule.legacyFixedMode') }}</p>
      <div class="grid gap-3 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800 sm:grid-cols-2">
        <div data-testid="video-row-cost">
          <p class="mb-1 font-medium">{{ t('videoModule.upstreamCost') }}</p>
          <template v-if="row.cost">
            <p class="mb-1 text-gray-500" data-testid="video-row-cost-date">{{ t(row.costFromCatalog ? 'videoModule.catalogCostRead' : 'videoModule.savedCostRead') }}: <time v-if="row.costAt" :datetime="row.costAt">{{ date(row.costAt) }}</time><span v-else>{{ t('videoModule.neverSynced') }}</span></p>
            <p>{{ t('videoModule.generationPrice') }}: {{ money(row.cost.unit_price) }} / {{ unit(row.cost.billing_mode) }}</p>
            <p>{{ t('videoModule.referencePrice') }}: {{ money(row.cost.reference_per_second) }} / {{ unit('video') }}</p>
            <p v-for="(price, resolution) in row.cost.resolution_prices" :key="resolution">{{ resolution }}: {{ money(price) }} / {{ unit(row.cost.billing_mode) }}</p>
            <p v-if="row.cost.unavailable_reason" class="text-amber-700 dark:text-amber-400">{{ row.cost.unavailable_reason }}</p>
          </template>
          <p v-else>{{ t('videoModule.costUnavailable') }}</p>
        </div>
        <div data-testid="video-row-customer">
          <p class="mb-1 font-medium">{{ t(row.customer.preview ? 'videoModule.customerPreview' : 'videoModule.customerEffective') }}</p>
          <p>{{ t('videoModule.generationPrice') }}: {{ money(row.customer.unit) }} / {{ unit(row.entry.billing_mode) }}</p>
          <template v-if="row.entry.video">
            <p>{{ t('videoModule.referencePrice') }}: {{ money(row.customer.reference) }} / {{ unit('video') }}</p>
            <p v-for="resolution in row.resolutions" :key="resolution">{{ resolution }}: {{ money(row.customer.resolutions[resolution] ?? row.customer.unit) }} / {{ unit(row.entry.billing_mode) }}</p>
          </template>
        </div>
      </div>
      <div v-if="row.entry.video" class="space-y-3 text-xs">
        <label class="flex items-center gap-2"><input :checked="row.upstream && row.entry.video.auto_price" :disabled="!row.upstream" type="checkbox" data-testid="video-auto-price" @change="updateVideo(index, { auto_price: ($event.target as HTMLInputElement).checked })">{{ t('videoModule.autoPrice') }}</label>
        <p class="text-gray-500">{{ t('videoModule.globalAutoSync') }}: {{ state(settings?.auto_sync) }} · {{ t('videoModule.lastPriceSync') }}: {{ date(row.entry.video.synced_at) }}</p>
        <fieldset :disabled="!row.upstream" class="space-y-3 disabled:opacity-60">
          <div class="flex flex-wrap gap-3">
            <label class="flex items-center gap-2"><input :checked="!!row.entry.video.markup" type="checkbox" data-testid="video-markup-override" @change="overrideMarkup(index, ($event.target as HTMLInputElement).checked)">{{ t('videoModule.overrideMarkup') }}</label>
            <label>{{ t('videoModule.followIncrease') }} <select class="input mt-1" data-testid="video-follow-increase" :value="row.entry.video.follow_increase === undefined ? '' : String(row.entry.video.follow_increase)" @change="updateVideo(index, { follow_increase: followValue($event) })"><option value="">{{ t('videoModule.inheritGlobal') }} ({{ state(settings?.follow_increase) }})</option><option value="true">{{ t('videoModule.enabled') }}</option><option value="false">{{ t('videoModule.disabled') }}</option></select></label>
            <label>{{ t('videoModule.followDecrease') }} <select class="input mt-1" data-testid="video-follow-decrease" :value="row.entry.video.follow_decrease === undefined ? '' : String(row.entry.video.follow_decrease)" @change="updateVideo(index, { follow_decrease: followValue($event) })"><option value="">{{ t('videoModule.inheritGlobal') }} ({{ state(settings?.follow_decrease) }})</option><option value="true">{{ t('videoModule.enabled') }}</option><option value="false">{{ t('videoModule.disabled') }}</option></select></label>
          </div>
          <p class="text-gray-500">{{ t(row.entry.video.markup ? 'videoModule.modelMarkup' : 'videoModule.inheritGlobal') }}</p>
          <div class="grid gap-3 sm:grid-cols-3">
            <label v-for="field in markupFields" :key="field">{{ t(`videoModule.markupFields.${field}`) }}
              <input :value="row.markup[field]" :readonly="!row.entry.video.markup" class="input mt-1" type="number" min="0" step="any" required :data-testid="`video-markup-${field}`" @input="setMarkup(index, field, $event)">
            </label>
          </div>
        </fieldset>
        <div v-if="!row.upstream" class="grid gap-3 sm:grid-cols-3">
          <label>{{ t('videoModule.manualReferencePrice') }} ({{ t('videoModule.accountingUnit') }} / {{ unit('video') }})<input :value="row.entry.video.reference_per_second" class="input mt-1" type="number" min="0" step="any" required data-testid="video-reference-price" @input="updateVideo(index, { reference_per_second: inputNumber($event) })"></label>
          <label v-for="resolution in row.resolutions" :key="resolution">{{ resolution }} ({{ t('videoModule.accountingUnit') }} / {{ unit(row.entry.billing_mode) }})<input :value="row.customer.resolutions[resolution] ?? row.customer.unit" class="input mt-1" type="number" min="0" step="any" required :data-testid="`video-resolution-${resolution}`" @input="updateVideo(index, { resolution_prices: { ...row.entry.video.resolution_prices, [resolution]: inputNumber($event) } })"></label>
        </div>
        <p class="text-gray-500">{{ t('videoModule.sourceHint') }}</p>
      </div>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('videoModule.priceHint') }}</p>
  </section>
</template>
