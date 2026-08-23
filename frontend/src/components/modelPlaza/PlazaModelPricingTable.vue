<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { UserPricingInterval } from '@/api/channels'
import type { PlazaModel } from '@/api/modelPlaza'
import Icon from '@/components/icons/Icon.vue'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode,
} from '@/constants/channel'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'

import PlazaPricePair from './PlazaPricePair.vue'

const props = withDefaults(defineProps<{
  models: PlazaModel[]
  platform?: string
  rateMultiplier: number
  userRateMultiplier?: number | null
  imageRateIndependent?: boolean
  imageRateMultiplier?: number
  videoRateIndependent?: boolean
  videoRateMultiplier?: number
}>(), {
  platform: '',
  userRateMultiplier: null,
  imageRateIndependent: false,
  imageRateMultiplier: 1,
  videoRateIndependent: false,
  videoRateMultiplier: 1,
})

const { t } = useI18n()
const PER_MILLION = 1_000_000
const VIDEO_DURATIONS = [5, 10, 15] as const
const usesPersonalTextRate = computed(() => (
  props.userRateMultiplier != null
  && Math.abs(props.userRateMultiplier - props.rateMultiplier) > Number.EPSILON
))

interface PricePair {
  reference: number | null
  charged: number | null
}

interface TokenPriceRow {
  key: string
  model: string
  platform: string
  tier: string
  input: PricePair
  output: PricePair
  cacheWrite: PricePair
  cacheRead: PricePair
  savings: number | null
}

interface UnitPriceRow {
  key: string
  model: string
  platform: string
  tier: string
  price: PricePair
  finalPrice: boolean
  savings: number | null
}

function price(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null
}

function billingMode(model: PlazaModel): BillingMode {
  return (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function effectiveRate(model: PlazaModel): number {
  switch (billingMode(model)) {
    case BILLING_MODE_IMAGE:
      if (props.imageRateIndependent) return props.imageRateMultiplier
      break
    case BILLING_MODE_VIDEO:
      if (props.videoRateIndependent) return props.videoRateMultiplier
      break
  }
  return props.userRateMultiplier ?? props.rateMultiplier
}

function pair(
  baseValue: number | null | undefined,
  officialValue: number | null | undefined,
  rate: number
): PricePair {
  const base = price(baseValue) ?? price(officialValue)
  return {
    reference: price(officialValue) ?? base,
    charged: base == null ? null : base * rate,
  }
}

function savingsFor(...pairs: PricePair[]): number | null {
  for (const item of pairs) {
    if (
      item.reference == null
      || item.charged == null
      || item.reference <= 0
      || item.charged >= item.reference
    ) continue
    return Math.max(0, Math.min(99, Math.round((1 - item.charged / item.reference) * 100)))
  }
  return null
}

function tierLabel(interval: UserPricingInterval): string {
  if (interval.tier_label) return interval.tier_label
  if (interval.max_tokens == null) {
    return interval.min_tokens > 0 ? `>${formatTokenCount(interval.min_tokens)}` : '-'
  }
  if (interval.min_tokens === 0) return `≤${formatTokenCount(interval.max_tokens)}`
  return `${formatTokenCount(interval.min_tokens)}-${formatTokenCount(interval.max_tokens)}`
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) return `${trimNumber(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimNumber(value / 1_000)}K`
  return String(value)
}

function trimNumber(value: number): string {
  return String(Math.round(value * 100) / 100)
}

function officialCacheWrite(model: PlazaModel, tier: string): number | null {
  if (/1\s*h|1\s*hour/i.test(tier)) {
    return price(model.official_pricing?.cache_write_1h_price)
      ?? price(model.official_pricing?.cache_write_price)
  }
  return price(model.official_pricing?.cache_write_price)
}

function tokenRowsFor(model: PlazaModel): TokenPriceRow[] {
  const rate = effectiveRate(model)
  const intervals = model.pricing?.intervals ?? []
  const buildRow = (interval: UserPricingInterval | null, index: number): TokenPriceRow => {
    const tier = interval ? tierLabel(interval) : '-'
    const input = pair(
      interval?.input_price ?? model.pricing?.input_price,
      model.official_pricing?.input_price,
      rate
    )
    const output = pair(
      interval?.output_price ?? model.pricing?.output_price,
      model.official_pricing?.output_price,
      rate
    )
    const cacheWrite = pair(
      interval?.cache_write_price ?? model.pricing?.cache_write_price,
      officialCacheWrite(model, tier),
      rate
    )
    const cacheRead = pair(
      interval?.cache_read_price ?? model.pricing?.cache_read_price,
      model.official_pricing?.cache_read_price,
      rate
    )
    return {
      key: `${model.platform}:${model.name}:${index}`,
      model: model.name,
      platform: model.platform,
      tier,
      input,
      output,
      cacheWrite,
      cacheRead,
      savings: savingsFor(output, input, cacheRead, cacheWrite),
    }
  }
  return intervals.length ? intervals.map(buildRow) : [buildRow(null, 0)]
}

function unitRowsFor(model: PlazaModel): UnitPriceRow[] {
  const defaultRate = effectiveRate(model)
  const finalTierValues = billingMode(model) === BILLING_MODE_VIDEO
    ? model.final_video_price_tiers
    : model.final_image_price_tiers
  const finalTiers = new Set(
    (finalTierValues ?? []).map((tier) => tier.trim().toUpperCase())
  )
  const intervals = (model.pricing?.intervals ?? []).filter(
    (interval) => interval.per_request_price != null
  )
  const buildRow = (interval: UserPricingInterval | null, index: number): UnitPriceRow => {
    const tier = interval ? tierLabel(interval) : '-'
    const finalPrice = finalTiers.has(tier.trim().toUpperCase())
    const rate = finalPrice ? 1 : defaultRate
    const base = price(interval?.per_request_price ?? model.pricing?.per_request_price)
    const pricePair = finalPrice
      ? { reference: null, charged: base }
      : pair(base, base, rate)
    return {
      key: `${model.platform}:${model.name}:${index}`,
      model: model.name,
      platform: model.platform,
      tier,
      price: pricePair,
      finalPrice,
      savings: savingsFor(pricePair),
    }
  }
  return intervals.length ? intervals.map(buildRow) : [buildRow(null, 0)]
}

const sortedModels = computed(() => [...props.models].sort((a, b) => {
  const aPrice = a.official_pricing?.output_price ?? a.pricing?.output_price ?? null
  const bPrice = b.official_pricing?.output_price ?? b.pricing?.output_price ?? null
  if (aPrice != null && bPrice != null && aPrice !== bPrice) return bPrice - aPrice
  if (aPrice != null && bPrice == null) return -1
  if (aPrice == null && bPrice != null) return 1
  return b.name.localeCompare(a.name)
}))

const textRows = computed(() => (
  sortedModels.value
    .filter((model) => billingMode(model) === BILLING_MODE_TOKEN)
    .flatMap(tokenRowsFor)
))
const imageRows = computed(() => (
  sortedModels.value
    .filter((model) => billingMode(model) === BILLING_MODE_IMAGE)
    .flatMap(unitRowsFor)
))
const videoRows = computed(() => (
  sortedModels.value
    .filter((model) => billingMode(model) === BILLING_MODE_VIDEO)
    .flatMap(unitRowsFor)
))
const requestRows = computed(() => (
  sortedModels.value
    .filter((model) => billingMode(model) === BILLING_MODE_PER_REQUEST)
    .flatMap(unitRowsFor)
))

const hasTextTiers = computed(() => textRows.value.some((row) => row.tier !== '-'))
const hasImageTiers = computed(() => imageRows.value.some((row) => row.tier !== '-'))
const hasVideoTiers = computed(() => videoRows.value.some((row) => row.tier !== '-'))
const hasRequestTiers = computed(() => requestRows.value.some((row) => row.tier !== '-'))

function durationPair(row: UnitPriceRow, seconds: number): PricePair {
  return {
    reference: row.price.reference == null ? null : row.price.reference * seconds,
    charged: row.price.charged == null ? null : row.price.charged * seconds,
  }
}
</script>

<template>
  <div class="pricing-ledger">
    <div class="price-legend">
      <span>
        <i class="customer-swatch"></i>
        {{ t(usesPersonalTextRate ? 'modelPlaza.table.personalPrice' : 'modelPlaza.table.paidPrice') }}
      </span>
      <span><i class="reference-swatch"></i>{{ t('modelPlaza.table.officialPrice') }}</span>
    </div>

    <section v-if="textRows.length" class="pricing-section">
      <header class="pricing-section-header">
        <strong>{{ t('modelPlaza.table.textModels') }}</strong>
        <span>{{ t('modelPlaza.table.cnyPerMillion') }}</span>
      </header>
      <div class="pricing-scroll">
        <table class="pricing-table text-table">
          <thead>
            <tr>
              <th>{{ t('modelPlaza.table.model') }}</th>
              <th v-if="hasTextTiers">{{ t('modelPlaza.table.tier') }}</th>
              <th>
                <span>{{ t('modelPlaza.table.input') }}</span>
                <small>{{ t('modelPlaza.table.cnyPerMillion') }}</small>
              </th>
              <th>
                <span>{{ t('modelPlaza.table.output') }}</span>
                <small>{{ t('modelPlaza.table.cnyPerMillion') }}</small>
              </th>
              <th>
                <span>{{ t('modelPlaza.table.cacheWrite') }}</span>
                <small>{{ t('modelPlaza.table.cnyPerMillion') }}</small>
              </th>
              <th>
                <span>{{ t('modelPlaza.table.cacheRead') }}</span>
                <small>{{ t('modelPlaza.table.cnyPerMillion') }}</small>
              </th>
              <th>{{ t('modelPlaza.table.savingsColumn') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in textRows" :key="row.key">
              <td class="model-name">
                <div class="model-identity">
                  <span>{{ row.model }}</span>
                  <span
                    v-if="props.platform && row.platform !== props.platform"
                    :class="['platform-badge', platformBadgeLightClass(row.platform)]"
                  >
                    {{ platformLabel(row.platform) }}
                  </span>
                </div>
              </td>
              <td v-if="hasTextTiers" class="tier-value">{{ row.tier }}</td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.input.reference"
                  :charged-price="row.input.charged"
                  :scale="PER_MILLION"
                />
              </td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.output.reference"
                  :charged-price="row.output.charged"
                  :scale="PER_MILLION"
                />
              </td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.cacheWrite.reference"
                  :charged-price="row.cacheWrite.charged"
                  :scale="PER_MILLION"
                />
              </td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.cacheRead.reference"
                  :charged-price="row.cacheRead.charged"
                  :scale="PER_MILLION"
                />
              </td>
              <td class="savings-cell">
                <span v-if="row.savings != null" class="savings-badge">
                  {{ t('modelPlaza.table.savings', { percent: row.savings }) }}
                </span>
                <span v-else class="no-savings">-</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-if="imageRows.length" class="pricing-section">
      <header class="pricing-section-header">
        <strong>{{ t('modelPlaza.table.imageModels') }}</strong>
        <span>{{ t('modelPlaza.table.cnyPerImage') }}</span>
      </header>
      <div class="pricing-scroll">
        <table class="pricing-table unit-table">
          <thead>
            <tr>
              <th>{{ t('modelPlaza.table.model') }}</th>
              <th v-if="hasImageTiers">{{ t('modelPlaza.table.tier') }}</th>
              <th>{{ t('modelPlaza.table.perImagePrice') }}</th>
              <th>{{ t('modelPlaza.table.savingsColumn') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in imageRows" :key="row.key">
              <td class="model-name">
                <div class="model-identity">
                  <span>{{ row.model }}</span>
                  <span
                    v-if="props.platform && row.platform !== props.platform"
                    :class="['platform-badge', platformBadgeLightClass(row.platform)]"
                  >{{ platformLabel(row.platform) }}</span>
                </div>
              </td>
              <td v-if="hasImageTiers" class="tier-value">{{ row.tier }}</td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.price.reference"
                  :charged-price="row.price.charged"
                  price-currency="usd"
                />
              </td>
              <td class="savings-cell">
                <span v-if="row.savings != null" class="savings-badge">
                  {{ t('modelPlaza.table.savings', { percent: row.savings }) }}
                </span>
                <span v-else class="no-savings">-</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-if="videoRows.length" class="pricing-section">
      <header class="pricing-section-header">
        <strong>{{ t('modelPlaza.table.videoModels') }}</strong>
        <span>{{ t('modelPlaza.table.videoPricingUnits') }}</span>
      </header>
      <div class="pricing-scroll">
        <table class="pricing-table video-table">
          <thead>
            <tr>
              <th>{{ t('modelPlaza.table.model') }}</th>
              <th v-if="hasVideoTiers">{{ t('modelPlaza.table.tier') }}</th>
              <th>{{ t('modelPlaza.table.videoUnitPrice') }}</th>
              <th>{{ t('modelPlaza.table.fiveSeconds') }}</th>
              <th>{{ t('modelPlaza.table.tenSeconds') }}</th>
              <th>{{ t('modelPlaza.table.fifteenSeconds') }}</th>
              <th>{{ t('modelPlaza.table.savingsColumn') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in videoRows" :key="row.key">
              <td class="model-name">
                <div class="model-identity">
                  <span>{{ row.model }}</span>
                  <span
                    v-if="props.platform && row.platform !== props.platform"
                    :class="['platform-badge', platformBadgeLightClass(row.platform)]"
                  >{{ platformLabel(row.platform) }}</span>
                </div>
              </td>
              <td v-if="hasVideoTiers" class="tier-value">{{ row.tier }}</td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.price.reference"
                  :charged-price="row.price.charged"
                  price-currency="usd"
                />
                <small class="video-price-unit">
                  {{ t(row.finalPrice ? 'modelPlaza.table.finalPerSecond' : 'modelPlaza.table.perSecondFallback') }}
                </small>
              </td>
              <td v-for="seconds in VIDEO_DURATIONS" :key="seconds" class="price-cell">
                <PlazaPricePair
                  :reference-price="durationPair(row, seconds).reference"
                  :charged-price="durationPair(row, seconds).charged"
                  price-currency="usd"
                />
              </td>
              <td class="savings-cell">
                <span v-if="row.savings != null" class="savings-badge">
                  {{ t('modelPlaza.table.savings', { percent: row.savings }) }}
                </span>
                <span v-else class="no-savings">-</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-if="requestRows.length" class="pricing-section">
      <header class="pricing-section-header">
        <strong>{{ t('modelPlaza.table.requestModels') }}</strong>
        <span>{{ t('modelPlaza.table.cnyPerRequest') }}</span>
      </header>
      <div class="pricing-scroll">
        <table class="pricing-table unit-table">
          <thead>
            <tr>
              <th>{{ t('modelPlaza.table.model') }}</th>
              <th v-if="hasRequestTiers">{{ t('modelPlaza.table.tier') }}</th>
              <th>{{ t('modelPlaza.table.perRequest') }}</th>
              <th>{{ t('modelPlaza.table.savingsColumn') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in requestRows" :key="row.key">
              <td class="model-name">
                <div class="model-identity">
                  <span>{{ row.model }}</span>
                  <span
                    v-if="props.platform && row.platform !== props.platform"
                    :class="['platform-badge', platformBadgeLightClass(row.platform)]"
                  >{{ platformLabel(row.platform) }}</span>
                </div>
              </td>
              <td v-if="hasRequestTiers" class="tier-value">{{ row.tier }}</td>
              <td class="price-cell">
                <PlazaPricePair
                  :reference-price="row.price.reference"
                  :charged-price="row.price.charged"
                />
              </td>
              <td class="savings-cell">
                <span v-if="row.savings != null" class="savings-badge">
                  {{ t('modelPlaza.table.savings', { percent: row.savings }) }}
                </span>
                <span v-else class="no-savings">-</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <footer class="pricing-footnote">
      <Icon name="infoCircle" size="xs" />
      {{ t('modelPlaza.table.actualPriceNote') }}
    </footer>
  </div>
</template>

<style scoped>
.pricing-ledger {
  width: 100%;
}

.price-legend {
  display: flex;
  min-height: 40px;
  align-items: center;
  justify-content: flex-end;
  gap: 16px;
  padding: 7px 22px;
  border-bottom: 1px solid var(--plaza-line-soft);
  color: var(--plaza-muted);
  background: var(--plaza-surface);
  font-size: 9px;
}

.price-legend span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.price-legend i {
  width: 16px;
  height: 3px;
  border-radius: 2px;
}

.reference-swatch {
  background: var(--plaza-reference);
}

.customer-swatch {
  background: var(--plaza-price);
}

.pricing-section + .pricing-section {
  border-top: 1px solid var(--plaza-line);
}

.pricing-section-header {
  display: flex;
  min-height: 40px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 22px;
  color: var(--plaza-muted);
  background: var(--plaza-surface-soft);
  font-size: 9px;
}

.pricing-section-header strong {
  color: var(--plaza-text-strong);
  font-size: 11px;
}

.pricing-scroll {
  overflow-x: auto;
}

.pricing-table {
  width: 100%;
  min-width: 700px;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 10px;
}

.text-table {
  min-width: 920px;
}

.video-table {
  min-width: 920px;
}

.unit-table {
  min-width: 620px;
}

.pricing-table th {
  padding: 10px 12px;
  color: var(--plaza-muted);
  background: var(--plaza-surface);
  font-size: 9px;
  font-weight: 650;
  text-align: center;
  vertical-align: bottom;
}

.pricing-table th:first-child {
  width: 25%;
  text-align: left;
}

.pricing-table th:last-child {
  width: 10%;
}

.pricing-table th > span,
.pricing-table th > small {
  display: block;
}

.pricing-table th > small {
  margin-top: 3px;
  color: var(--plaza-muted-soft);
  font-size: 8px;
  font-weight: 500;
}

.pricing-table td {
  min-height: 60px;
  padding: 8px 12px;
  border-top: 1px solid var(--plaza-line-soft);
  color: var(--plaza-text);
  vertical-align: middle;
}

.pricing-table tbody tr:nth-child(even) {
  background: color-mix(in srgb, var(--plaza-surface-muted) 58%, transparent);
}

.pricing-table tbody tr:hover {
  background: var(--plaza-hover);
}

.model-name {
  color: var(--plaza-text-strong) !important;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.model-identity {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.platform-badge {
  display: inline-flex;
  min-height: 19px;
  align-items: center;
  border-radius: 4px;
  padding: 0 5px;
  font-family: ui-sans-serif, system-ui, sans-serif;
  font-size: 8px;
  font-weight: 700;
}

.tier-value {
  color: var(--plaza-muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 9px;
  text-align: center;
}

.price-cell {
  text-align: center;
}

.price-cell :deep(.price-pair) {
  justify-items: center;
}

.video-price-unit {
  display: block;
  margin-top: -2px;
  color: var(--plaza-muted);
  font-size: 8px;
  white-space: nowrap;
}

.savings-cell {
  text-align: center;
}

.savings-badge {
  display: inline-flex;
  min-height: 25px;
  align-items: center;
  justify-content: center;
  border: 1px solid color-mix(in srgb, var(--plaza-success) 36%, transparent);
  border-radius: 5px;
  padding: 0 7px;
  color: var(--plaza-success);
  background: var(--plaza-success-bg);
  font-size: 9px;
  font-weight: 750;
  white-space: nowrap;
}

.no-savings {
  color: var(--plaza-muted-soft);
}

.pricing-footnote {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 10px 22px;
  border-top: 1px solid var(--plaza-line-soft);
  color: var(--plaza-muted);
  background: var(--plaza-surface-muted);
  font-size: 9px;
  line-height: 1.5;
}

.pricing-footnote :deep(svg) {
  flex: 0 0 auto;
  margin-top: 1px;
}

@media (max-width: 640px) {
  .price-legend {
    justify-content: flex-start;
    overflow-x: auto;
    padding-inline: 14px;
  }

  .pricing-section-header,
  .pricing-footnote {
    padding-inline: 14px;
  }

  .pricing-table th,
  .pricing-table td {
    padding-inline: 10px;
  }
}
</style>
