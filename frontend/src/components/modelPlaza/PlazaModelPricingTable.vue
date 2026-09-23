<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { UserPricingInterval } from '@/api/channels'
import type { PlazaModel, PlazaTimePricingPeriod } from '@/api/modelPlaza'
import Icon from '@/components/icons/Icon.vue'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  REASONING_EFFORT_LEVELS,
  type BillingMode,
} from '@/constants/channel'
import { platformAccentColor, platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { resolveIntervalPrices } from '@/utils/pricing'

import PlazaPricePair from './PlazaPricePair.vue'

const props = withDefaults(defineProps<{
  models: PlazaModel[]
  platform?: string
  rateMultiplier: number
  userRateMultiplier?: number | null
  imageRateIndependent?: boolean
  imageRateMultiplier?: number | null
  videoRateIndependent?: boolean
  videoRateMultiplier?: number | null
  peakWindow?: string
  peakRateMultiplier?: number | null
}>(), {
  platform: '',
  userRateMultiplier: null,
  imageRateIndependent: false,
  imageRateMultiplier: 1,
  videoRateIndependent: false,
  videoRateMultiplier: 1,
  peakWindow: '',
  peakRateMultiplier: null,
})

const { t } = useI18n()
const PER_MILLION = 1_000_000
const VIDEO_DURATIONS = [5, 10, 15] as const
const accentStyle = computed(() => ({ '--plaza-accent': platformAccentColor(props.platform ?? '') }))
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
  period: PlazaTimePricingPeriod | null
  longContextBasis?: PlazaModel['long_context_basis']
  reasoningEffortMultipliers: [string, number][]
  input: PricePair
  output: PricePair
  cacheWrite: PricePair
  cacheWrite1h: PricePair
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

interface TokenIntervalPair {
  pricing: UserPricingInterval | null
  official: UserPricingInterval | null
}

function price(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null
}

function billingMode(model: PlazaModel): BillingMode {
  return (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function billingModeRank(model: PlazaModel): number {
  switch (billingMode(model)) {
    case BILLING_MODE_TOKEN:
      return 0
    case BILLING_MODE_IMAGE:
      return 1
    case BILLING_MODE_VIDEO:
      return 2
    case BILLING_MODE_PER_REQUEST:
      return 3
    default:
      return 4
  }
}

function effectiveRate(model: PlazaModel, period: PlazaTimePricingPeriod | null = null): number {
  let rate = props.userRateMultiplier ?? props.rateMultiplier
  switch (billingMode(model)) {
    case BILLING_MODE_IMAGE:
      if (props.imageRateIndependent) rate = props.imageRateMultiplier ?? 1
      break
    case BILLING_MODE_VIDEO:
      if (props.videoRateIndependent) rate = props.videoRateMultiplier ?? 1
      break
  }
  if (period) {
    return Math.round(rate * period.multiplier * 1000) / 1000
  }
  return rate
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

function sortByContext(intervals: UserPricingInterval[]): UserPricingInterval[] {
  return [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)
}

function tokenIntervals(model: PlazaModel): UserPricingInterval[] {
  if (!model.pricing) return []
  return sortByContext(model.pricing.intervals ?? []).map((interval) =>
    resolveIntervalPrices(interval, model.pricing!)
  )
}

function officialIntervals(model: PlazaModel): UserPricingInterval[] {
  if (!model.official_pricing) return []
  return sortByContext(model.official_pricing.intervals ?? []).map((interval) =>
    resolveIntervalPrices(interval, model.official_pricing!)
  )
}

function requestIntervals(model: PlazaModel): UserPricingInterval[] {
  return (model.pricing?.intervals ?? []).filter((interval) => interval.per_request_price != null)
}

function tokenIntervalPairs(model: PlazaModel): TokenIntervalPair[] {
  const pricing = tokenIntervals(model)
  const official = officialIntervals(model)
  if (pricing.length > 0) {
    return pricing.map((interval, index) => ({ pricing: interval, official: official[index] ?? null }))
  }
  if (official.length > 0) {
    return official.map((interval) => ({ pricing: null, official: interval }))
  }
  return [{ pricing: null, official: null }]
}

function tierLabel(interval: UserPricingInterval): string {
  if (interval.tier_label) return interval.tier_label
  const { min_tokens: min, max_tokens: max } = interval
  return max == null ? `>${formatTokenCount(min)}` : `≤${formatTokenCount(max)}`
}

function tokenPairLabel(pair: TokenIntervalPair): string {
  const interval = pair.pricing ?? pair.official
  return interval ? tierLabel(interval) : '-'
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) return `${trimNumber(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimNumber(value / 1_000)}K`
  return String(value)
}

function trimNumber(value: number): string {
  return String(Math.round(value * 100) / 100)
}

function timePeriods(model: PlazaModel): PlazaTimePricingPeriod[] {
  return model.time_pricing?.periods ?? []
}

function timePricingRowHint(model: PlazaModel): string {
  const key = model.time_pricing?.weekdays_only
    ? 'modelPlaza.table.timePricingRowHintWeekdays'
    : 'modelPlaza.table.timePricingRowHint'
  let hint = t(key, { timezone: model.time_pricing?.timezone })
  if (props.peakWindow) {
    hint += t('modelPlaza.table.timePricingRowHintPeak', {
      window: props.peakWindow,
      multiplier: props.peakRateMultiplier ?? 1
    })
  }
  return hint
}

function formatTimeWindow(period: PlazaTimePricingPeriod): string {
  const clock = (value: string) => value.replace(/^(\d{2}:\d{2}):00$/, '$1')
  return `${clock(period.start_time)}–${clock(period.end_time)}`
}

function buildTokenRow(
  model: PlazaModel,
  intervalPair: TokenIntervalPair,
  index: number,
  period: PlazaTimePricingPeriod | null
): TokenPriceRow {
  const pricing = intervalPair.pricing
  const official = intervalPair.official
  const rate = effectiveRate(model, period)
  const input = pair(
    pricing?.input_price ?? model.pricing?.input_price,
    official?.input_price ?? model.official_pricing?.input_price,
    rate
  )
  const output = pair(
    pricing?.output_price ?? model.pricing?.output_price,
    official?.output_price ?? model.official_pricing?.output_price,
    rate
  )
  const cacheWrite = pair(
    pricing?.cache_write_price ?? model.pricing?.cache_write_price,
    official?.cache_write_price ?? model.official_pricing?.cache_write_price,
    rate
  )
  const cacheWrite1h = pair(
    pricing?.cache_write_1h_price ?? model.pricing?.cache_write_1h_price,
    official?.cache_write_1h_price ?? model.official_pricing?.cache_write_1h_price,
    rate
  )
  const cacheRead = pair(
    pricing?.cache_read_price ?? model.pricing?.cache_read_price,
    official?.cache_read_price ?? model.official_pricing?.cache_read_price,
    rate
  )
  const periodKey = period ? `${period.start_time}-${period.end_time}-${period.multiplier}` : 'base'
  return {
    key: `${model.platform}:${model.name}:${periodKey}:${index}`,
    model: model.name,
    platform: model.platform,
    tier: tokenPairLabel(intervalPair),
    period,
    longContextBasis: model.long_context_basis,
    reasoningEffortMultipliers: REASONING_EFFORT_LEVELS.flatMap(effort => {
      const multiplier = model.pricing?.reasoning_effort_multipliers?.[effort]
      return typeof multiplier === 'number' && Number.isFinite(multiplier) && multiplier > 0
        ? [[effort, multiplier] as [string, number]] : []
    }),
    input,
    output,
    cacheWrite,
    cacheWrite1h,
    cacheRead,
    savings: savingsFor(output, input, cacheRead, cacheWrite, cacheWrite1h),
  }
}

function tokenRowsFor(model: PlazaModel): TokenPriceRow[] {
  const intervalPairs = tokenIntervalPairs(model)
  const baseRows = intervalPairs.map((intervalPair, index) => buildTokenRow(model, intervalPair, index, null))
  const periodRows = timePeriods(model).flatMap((period) =>
    intervalPairs.map((intervalPair, index) => buildTokenRow(model, intervalPair, index, period))
  )
  return [...baseRows, ...periodRows]
}

function unitRowsFor(model: PlazaModel): UnitPriceRow[] {
  const defaultRate = effectiveRate(model)
  const finalTierValues = billingMode(model) === BILLING_MODE_VIDEO
    ? model.final_video_price_tiers
    : model.final_image_price_tiers
  const finalTiers = new Set(
    (finalTierValues ?? []).map((tier) => tier.trim().toUpperCase())
  )
  const intervals = requestIntervals(model)
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
  const rank = billingModeRank(a) - billingModeRank(b)
  if (rank !== 0) return rank
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

function rowModel(row: TokenPriceRow): PlazaModel | undefined {
  return props.models.find((item) => item.name === row.model && item.platform === row.platform)
}

function rowTimePricingHint(row: TokenPriceRow): string {
  const model = rowModel(row)
  return model ? timePricingRowHint(model) : ''
}

function rowIsWeekdaysOnly(row: TokenPriceRow): boolean {
  return rowModel(row)?.time_pricing?.weekdays_only === true
}

function rowTierHint(row: TokenPriceRow): string | undefined {
  if (row.tier === '-') return undefined
  return row.longContextBasis === 'marginal'
    ? t('modelPlaza.table.tierHintMarginal')
    : t('modelPlaza.table.tierHint')
}

function durationPair(row: UnitPriceRow, seconds: number): PricePair {
  return {
    reference: row.price.reference == null ? null : row.price.reference * seconds,
    charged: row.price.charged == null ? null : row.price.charged * seconds,
  }
}
</script>

<template>
  <div class="pricing-ledger" :style="accentStyle">
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
                    v-for="([effort, multiplier]) in row.reasoningEffortMultipliers"
                    :key="effort"
                    :data-reasoning-effort="effort"
                    class="reasoning-badge"
                    :title="t('modelPlaza.table.reasoningMultiplierHint', { effort, multiplier })"
                  >
                    {{ t('modelPlaza.table.reasoningMultiplierBadge', { effort, multiplier }) }}
                  </span>
                  <span
                    v-if="row.period"
                    class="period-badge"
                    :title="rowTimePricingHint(row)"
                  >
                    <span v-if="rowIsWeekdaysOnly(row)">
                      {{ t('modelPlaza.table.timePricingWeekdays') }}
                    </span>
                    {{ formatTimeWindow(row.period) }}
                  </span>
                  <span
                    v-if="props.platform && row.platform !== props.platform"
                    :class="['platform-badge', platformBadgeLightClass(row.platform)]"
                  >
                    {{ platformLabel(row.platform) }}
                  </span>
                  <span
                    v-if="row.longContextBasis === 'marginal'"
                    class="context-badge"
                    :title="t('modelPlaza.table.tierHintMarginal')"
                  >
                    {{ t('modelPlaza.table.marginalBadge') }}
                  </span>
                </div>
              </td>
              <td v-if="hasTextTiers" class="tier-value">
                <span :title="rowTierHint(row)">
                  {{ row.tier }}
                </span>
              </td>
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
                <div class="price-stack">
                  <PlazaPricePair
                    :reference-price="row.cacheWrite.reference"
                    :charged-price="row.cacheWrite.charged"
                    :scale="PER_MILLION"
                  />
                  <div
                    v-if="row.cacheWrite1h.reference != null || row.cacheWrite1h.charged != null"
                    class="sub-price"
                  >
                    <span>1h</span>
                    <PlazaPricePair
                      :reference-price="row.cacheWrite1h.reference"
                      :charged-price="row.cacheWrite1h.charged"
                      :scale="PER_MILLION"
                    />
                  </div>
                </div>
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

.platform-badge,
.period-badge,
.context-badge,
.reasoning-badge {
  display: inline-flex;
  min-height: 19px;
  align-items: center;
  gap: 4px;
  border-radius: 4px;
  padding: 0 5px;
  font-family: ui-sans-serif, system-ui, sans-serif;
  font-size: 8px;
  font-weight: 700;
}

.period-badge,
.context-badge {
  color: var(--plaza-muted);
  background: var(--plaza-surface-soft);
}

.reasoning-badge {
  color: #a16207;
  background: #fef9c3;
}

.dark .reasoning-badge {
  color: #fde68a;
  background: rgb(120 53 15 / 32%);
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

.price-stack {
  display: grid;
  gap: 4px;
}

.sub-price {
  display: grid;
  gap: 2px;
}

.sub-price > span {
  color: var(--plaza-muted-soft);
  font-size: 8px;
  font-weight: 700;
}

.sub-price :deep(.customer-price) {
  font-size: 12px;
}

.sub-price :deep(.reference-price) {
  font-size: 8px;
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
