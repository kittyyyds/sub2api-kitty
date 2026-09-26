<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  useBillingEstimator,
  type BillingWorkloadPreset,
} from '@/composables/useBillingEstimator'

const { t, locale } = useI18n()
const {
  groups,
  selectedGroupId,
  selectedModelKey,
  selectedGroup,
  serviceTier,
  advancedOpen,
  catalogLoading,
  estimating,
  catalogError,
  previewError,
  result,
  tokens,
  hasCatalog,
  modelKey,
  loadCatalog,
  estimateNow,
  sanitizeToken,
  applyPreset,
  reset,
  toggleAdvanced,
} = useBillingEstimator()

const workloadPresets = computed<(BillingWorkloadPreset & { label: string })[]>(() => [
  { id: 'chat', label: t('native.billing.preset.chat'), input: 4_000, output: 1_200 },
  { id: 'coding', label: t('native.billing.preset.coding'), input: 90_000, output: 18_000 },
  { id: 'document', label: t('native.billing.preset.document'), input: 450_000, output: 22_000 },
])

const groupOptions = computed<SelectOption[]>(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: `${formatMultiplier(group.effective_rate_multiplier)}x`,
  })),
)

const modelOptions = computed<SelectOption[]>(() =>
  (selectedGroup.value?.models ?? []).map((model) => ({
    value: modelKey(model),
    label: model.name,
    description: model.platform,
  })),
)

const serviceTierOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('native.billing.calculator.tierStandard') },
  { value: 'priority', label: t('native.billing.calculator.tierPriority') },
  { value: 'flex', label: t('native.billing.calculator.tierFlex') },
])

const contributionRows = computed(() => {
  const breakdown = result.value?.charged_breakdown
  return [
    {
      id: 'input',
      label: t('native.billing.contribution.input'),
      amount: breakdown?.input_cost ?? 0,
      tone: 'cyan',
    },
    {
      id: 'output',
      label: t('native.billing.contribution.output'),
      amount: breakdown?.output_cost ?? 0,
      tone: 'copper',
    },
    {
      id: 'cache-write',
      label: t('native.billing.contribution.cacheWrite'),
      amount: breakdown?.cache_creation_cost ?? 0,
      tone: 'teal',
    },
    {
      id: 'cache-read',
      label: t('native.billing.contribution.cacheRead'),
      amount: breakdown?.cache_read_cost ?? 0,
      tone: 'steel',
    },
  ]
})

const maxContribution = computed(() =>
  Math.max(...contributionRows.value.map((row) => row.amount), 0),
)

function contributionStyle(amount: number): { width: string } {
  if (maxContribution.value === 0 || amount === 0) return { width: '0%' }
  return { width: `${Math.max(4, (amount / maxContribution.value) * 100).toFixed(2)}%` }
}

function formatCurrency(value: number): string {
  const maximumFractionDigits = value > 0 && value < 0.01 ? 6 : 4
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits,
  }).format(value)
}

function formatInteger(value: number): string {
  const parsed = Number(value)
  return new Intl.NumberFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    maximumFractionDigits: 0,
  }).format(Number.isFinite(parsed) ? Math.max(0, parsed) : 0)
}

function formatMultiplier(value: number): string {
  return new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 0,
    maximumFractionDigits: 4,
  }).format(value)
}

function pricingSourceLabel(source: string): string {
  const known = ['group', 'channel', 'litellm', 'fallback']
  return known.includes(source)
    ? t(`native.billing.calculator.pricingSource.${source}`)
    : source
}
</script>

<template>
  <div class="billing-estimator">
    <div v-if="catalogLoading" class="billing-estimator__state" aria-live="polite">
      <i class="billing-estimator__spinner"></i>
      <span>{{ t('native.billing.calculator.loadingCatalog') }}</span>
    </div>

    <div v-else-if="catalogError" class="billing-estimator__state billing-estimator__state--error">
      <Icon name="infoCircle" size="md" :stroke-width="1.8" />
      <div>
        <strong>{{ t('native.billing.calculator.catalogError') }}</strong>
        <button type="button" @click="loadCatalog">{{ t('native.billing.calculator.retry') }}</button>
      </div>
    </div>

    <div v-else-if="!hasCatalog" class="billing-estimator__state">
      <Icon name="infoCircle" size="md" :stroke-width="1.8" />
      <span>{{ t('native.billing.calculator.noCatalog') }}</span>
    </div>

    <div v-else class="billing-estimator__tool">
      <div class="billing-estimator__form">
        <div class="billing-estimator__toolbar">
          <div class="workload-presets" :aria-label="t('native.billing.calculator.workloadAria')">
            <span>{{ t('native.billing.calculator.workload') }}</span>
            <button
              v-for="preset in workloadPresets"
              :key="preset.id"
              type="button"
              @click="applyPreset(preset)"
            >
              {{ preset.label }}
            </button>
          </div>
          <button
            class="reset-button"
            type="button"
            :title="t('native.billing.calculator.resetTitle')"
            @click="reset"
          >
            <Icon name="refresh" size="sm" :stroke-width="1.8" />
            <span>{{ t('native.billing.calculator.reset') }}</span>
          </button>
        </div>

        <div class="billing-estimator__selection">
          <label class="estimator-field">
            <span>{{ t('native.billing.calculator.group') }}</span>
            <Select
              v-model="selectedGroupId"
              data-testid="billing-group"
              :options="groupOptions"
              :searchable="groupOptions.length > 5"
              :placeholder="t('native.billing.calculator.groupPlaceholder')"
              :aria-label="t('native.billing.calculator.group')"
            />
          </label>

          <label class="estimator-field">
            <span>{{ t('native.billing.calculator.model') }}</span>
            <Select
              v-model="selectedModelKey"
              data-testid="billing-model"
              :options="modelOptions"
              :searchable="modelOptions.length > 5"
              :disabled="modelOptions.length === 0"
              :placeholder="t('native.billing.calculator.modelPlaceholder')"
              :aria-label="t('native.billing.calculator.model')"
            />
          </label>
        </div>

        <div v-if="selectedGroup" class="billing-estimator__rate-strip">
          <div>
            <span>{{ t('native.billing.calculator.groupRate') }}</span>
            <strong>{{ formatMultiplier(selectedGroup.rate_multiplier) }}x</strong>
          </div>
          <span class="billing-estimator__rate-operator">×</span>
          <div>
            <span>{{ t('native.billing.calculator.peakRate') }}</span>
            <strong>{{ formatMultiplier(selectedGroup.peak_multiplier) }}x</strong>
          </div>
          <span class="billing-estimator__rate-operator">=</span>
          <div class="billing-estimator__rate-total">
            <span>{{ t('native.billing.calculator.effectiveRate') }}</span>
            <strong>{{ formatMultiplier(selectedGroup.effective_rate_multiplier) }}x</strong>
          </div>
          <em v-if="selectedGroup.rate_source === 'user'">
            {{ t('native.billing.calculator.personalRate') }}
          </em>
        </div>

        <div class="billing-estimator__tokens">
          <label class="estimator-field">
            <span>{{ t('native.billing.calculator.inputTokens') }}</span>
            <input
              v-model.number="tokens.input"
              data-testid="input-tokens"
              type="number"
              inputmode="numeric"
              min="0"
              max="1000000000"
              step="1000"
              @blur="sanitizeToken('input')"
            />
            <small>{{ formatInteger(tokens.input) }} token</small>
          </label>

          <label class="estimator-field">
            <span>{{ t('native.billing.calculator.outputTokens') }}</span>
            <input
              v-model.number="tokens.output"
              data-testid="output-tokens"
              type="number"
              inputmode="numeric"
              min="0"
              max="1000000000"
              step="1000"
              @blur="sanitizeToken('output')"
            />
            <small>{{ formatInteger(tokens.output) }} token</small>
          </label>
        </div>

        <button
          class="billing-estimator__advanced-toggle"
          type="button"
          :aria-expanded="advancedOpen"
          @click="toggleAdvanced"
        >
          <Icon
            name="chevronDown"
            size="sm"
            :class="advancedOpen && 'billing-estimator__advanced-icon--open'"
          />
          <span>{{ t('native.billing.calculator.advanced') }}</span>
        </button>

        <Transition name="advanced-fields">
          <div v-if="advancedOpen" class="billing-estimator__advanced">
            <label class="estimator-field">
              <span>{{ t('native.billing.calculator.cacheWrite5m') }}</span>
              <input
                v-model.number="tokens.cacheWrite5m"
                data-testid="cache-write-5m-tokens"
                type="number"
                inputmode="numeric"
                min="0"
                max="1000000000"
                step="1000"
                @blur="sanitizeToken('cacheWrite5m')"
              />
            </label>
            <label class="estimator-field">
              <span>{{ t('native.billing.calculator.cacheWrite1h') }}</span>
              <input
                v-model.number="tokens.cacheWrite1h"
                data-testid="cache-write-1h-tokens"
                type="number"
                inputmode="numeric"
                min="0"
                max="1000000000"
                step="1000"
                @blur="sanitizeToken('cacheWrite1h')"
              />
            </label>
            <label class="estimator-field">
              <span>{{ t('native.billing.calculator.cacheRead') }}</span>
              <input
                v-model.number="tokens.cacheRead"
                data-testid="cache-read-tokens"
                type="number"
                inputmode="numeric"
                min="0"
                max="1000000000"
                step="1000"
                @blur="sanitizeToken('cacheRead')"
              />
            </label>
            <label class="estimator-field">
              <span>{{ t('native.billing.calculator.serviceTier') }}</span>
              <Select
                v-model="serviceTier"
                data-testid="service-tier"
                :options="serviceTierOptions"
                :searchable="false"
                :aria-label="t('native.billing.calculator.serviceTier')"
              />
            </label>
          </div>
        </Transition>
      </div>

      <aside class="billing-estimator__result" aria-live="polite" :aria-busy="estimating">
        <div class="billing-estimator__result-heading">
          <span>{{ t('native.billing.calculator.estimate') }}</span>
          <i v-if="estimating" :title="t('native.billing.calculator.calculating')"></i>
          <b v-else-if="result">{{ pricingSourceLabel(result.pricing_source) }}</b>
        </div>

        <strong data-testid="estimated-cost">
          {{ result ? formatCurrency(result.actual_cost) : '--' }}
        </strong>

        <p v-if="result" data-testid="billing-equation">
          {{ formatCurrency(result.standard_cost) }}
          <span>×</span>
          {{ formatMultiplier(result.effective_rate_multiplier) }}x
          <span>=</span>
          {{ formatCurrency(result.actual_cost) }}
        </p>
        <p v-else>{{ t('native.billing.calculator.waitingResult') }}</p>

        <div class="contribution-list">
          <div v-for="row in contributionRows" :key="row.id" class="contribution-row">
            <div class="contribution-row__meta">
              <span>{{ row.label }}</span>
              <b>{{ formatCurrency(row.amount) }}</b>
            </div>
            <div class="contribution-row__track">
              <i
                :class="`contribution-row__bar contribution-row__bar--${row.tone}`"
                :style="contributionStyle(row.amount)"
              ></i>
            </div>
          </div>
        </div>

        <div v-if="previewError" class="billing-estimator__notice billing-estimator__notice--error">
          <Icon name="infoCircle" size="sm" :stroke-width="1.8" />
          <span>{{ t('native.billing.calculator.previewError') }}</span>
          <button type="button" @click="estimateNow">{{ t('native.billing.calculator.retry') }}</button>
        </div>
        <div v-else-if="result?.long_context_applied" class="billing-estimator__notice">
          <Icon name="infoCircle" size="sm" :stroke-width="1.8" />
          <span>{{ t('native.billing.calculator.longContextApplied') }}</span>
        </div>
        <div v-else class="billing-estimator__notice">
          <Icon name="check" size="sm" :stroke-width="1.8" />
          <span>{{ t('native.billing.calculator.syncedRule') }}</span>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.billing-estimator {
  min-height: 320px;
}

.billing-estimator__state {
  display: flex;
  min-height: 260px;
  gap: 10px;
  align-items: center;
  justify-content: center;
  padding: 24px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 13px;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 8px;
}

.billing-estimator__state--error {
  color: var(--customer-copper, #9c641f);
}

.billing-estimator__state--error div {
  display: grid;
  gap: 6px;
}

.billing-estimator__state button,
.billing-estimator__notice button {
  width: fit-content;
  padding: 0;
  color: var(--customer-cyan, #0a7f90);
  font: inherit;
  font-weight: 650;
  cursor: pointer;
  background: transparent;
  border: 0;
}

.billing-estimator__spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-top-color: var(--customer-cyan, #0a7f90);
  border-radius: 50%;
  animation: estimator-spin 700ms linear infinite;
}

.billing-estimator__tool {
  display: grid;
  grid-template-columns: minmax(0, 1.55fr) minmax(290px, 0.75fr);
  overflow: hidden;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 8px;
  box-shadow: var(--customer-shadow, 0 14px 40px rgba(25, 47, 55, 0.1));
}

.billing-estimator__form {
  min-width: 0;
  padding: 20px;
}

.billing-estimator__toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.workload-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.workload-presets > span {
  margin-right: 2px;
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-size: 11px;
}

.workload-presets button,
.reset-button,
.billing-estimator__advanced-toggle {
  min-height: 30px;
  padding: 5px 9px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font: inherit;
  font-size: 11px;
  font-weight: 650;
  cursor: pointer;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 5px;
}

.workload-presets button:hover,
.reset-button:hover,
.billing-estimator__advanced-toggle:hover {
  color: var(--customer-cyan, #0a7f90);
  background: var(--customer-cyan-soft, rgba(10, 127, 144, 0.1));
  border-color: var(--customer-line-strong, rgba(10, 127, 144, 0.36));
}

.reset-button,
.billing-estimator__advanced-toggle {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
}

.billing-estimator__selection,
.billing-estimator__tokens {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.billing-estimator__selection {
  padding-top: 18px;
}

.billing-estimator__tokens {
  margin-top: 18px;
}

.estimator-field {
  display: block;
  min-width: 0;
}

.estimator-field > span {
  display: block;
  margin-bottom: 7px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 11px;
  font-weight: 650;
  line-height: 1.4;
}

.estimator-field input {
  width: 100%;
  height: 40px;
  min-width: 0;
  padding: 8px 10px;
  color: var(--customer-ink, #142126);
  font-family: var(--customer-mono, monospace);
  font-size: 13px;
  background: var(--customer-surface-raised, #f8fbfc);
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 5px;
}

.estimator-field input:hover {
  border-color: var(--customer-line-strong, rgba(10, 127, 144, 0.36));
}

.estimator-field small {
  display: block;
  min-height: 16px;
  margin-top: 5px;
  overflow: hidden;
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-size: 10px;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.estimator-field :deep(.select-trigger) {
  width: 100%;
  min-height: 40px;
  color: var(--customer-ink, #142126);
  background: var(--customer-surface-raised, #f8fbfc);
  border-color: var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 5px;
}

.estimator-field input:focus-visible,
.estimator-field :deep(.select-trigger:focus-visible),
.workload-presets button:focus-visible,
.reset-button:focus-visible,
.billing-estimator__advanced-toggle:focus-visible,
.billing-estimator__state button:focus-visible,
.billing-estimator__notice button:focus-visible {
  outline: 2px solid var(--customer-cyan, #0a7f90);
  outline-offset: 2px;
}

.billing-estimator__rate-strip {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto minmax(0, 1.15fr) auto;
  gap: 9px;
  align-items: center;
  padding: 11px 12px;
  margin-top: 14px;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border-left: 2px solid var(--customer-cyan, #0a7f90);
}

.billing-estimator__rate-strip div {
  min-width: 0;
}

.billing-estimator__rate-strip span,
.billing-estimator__rate-strip strong {
  display: block;
}

.billing-estimator__rate-strip span {
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-size: 9px;
}

.billing-estimator__rate-strip strong {
  margin-top: 2px;
  color: var(--customer-ink, #142126);
  font-family: var(--customer-mono, monospace);
  font-size: 12px;
}

.billing-estimator__rate-strip .billing-estimator__rate-operator {
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 12px;
}

.billing-estimator__rate-total strong {
  color: var(--customer-copper, #9c641f);
}

.billing-estimator__rate-strip em {
  padding: 3px 6px;
  color: var(--customer-copper, #9c641f);
  font-size: 9px;
  font-style: normal;
  font-weight: 650;
  white-space: nowrap;
  border: 1px solid color-mix(in srgb, var(--customer-copper, #9c641f) 35%, transparent);
}

.billing-estimator__advanced-toggle {
  margin-top: 16px;
}

.billing-estimator__advanced-toggle svg {
  transition: transform 180ms ease;
}

.billing-estimator__advanced-icon--open {
  transform: rotate(180deg);
}

.billing-estimator__advanced {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  padding-top: 14px;
  margin-top: 14px;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.billing-estimator__result {
  min-width: 0;
  padding: 24px;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border-left: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.billing-estimator__result-heading {
  display: flex;
  min-height: 18px;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-family: var(--customer-mono, monospace);
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
}

.billing-estimator__result-heading i {
  width: 8px;
  height: 8px;
  background: var(--customer-cyan, #0a7f90);
  border-radius: 50%;
  animation: estimator-pulse 900ms ease-in-out infinite;
}

.billing-estimator__result-heading b {
  color: var(--customer-cyan, #0a7f90);
  font-size: 9px;
}

.billing-estimator__result > strong {
  display: block;
  min-height: 40px;
  margin-top: 5px;
  overflow-wrap: anywhere;
  color: var(--customer-copper, #9c641f);
  font-family: var(--customer-mono, monospace);
  font-size: 31px;
  font-weight: 720;
  line-height: 1.25;
}

.billing-estimator__result > p {
  min-height: 18px;
  margin: 4px 0 20px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-family: var(--customer-mono, monospace);
  font-size: 11px;
}

.billing-estimator__result > p span {
  padding: 0 4px;
  color: var(--customer-cyan, #0a7f90);
}

.contribution-list {
  display: grid;
  gap: 14px;
}

.contribution-row__meta {
  display: flex;
  gap: 12px;
  justify-content: space-between;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 11px;
}

.contribution-row__meta b {
  color: var(--customer-ink, #142126);
  font-family: var(--customer-mono, monospace);
  font-weight: 650;
}

.contribution-row__track {
  height: 4px;
  margin-top: 6px;
  overflow: hidden;
  background: var(--customer-line, rgba(20, 33, 38, 0.12));
}

.contribution-row__bar {
  display: block;
  height: 100%;
  background: var(--customer-cyan, #0a7f90);
  transition: width 180ms ease;
}

.contribution-row__bar--copper {
  background: var(--customer-copper, #9c641f);
}

.contribution-row__bar--teal {
  background: #28756b;
}

.contribution-row__bar--steel {
  background: var(--customer-muted, rgba(20, 33, 38, 0.66));
}

.billing-estimator__notice {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding-top: 16px;
  margin-top: 20px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 10px;
  line-height: 1.55;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.billing-estimator__notice svg {
  flex: 0 0 auto;
  color: var(--customer-cyan, #0a7f90);
}

.billing-estimator__notice--error,
.billing-estimator__notice--error svg {
  color: var(--customer-copper, #9c641f);
}

.advanced-fields-enter-active,
.advanced-fields-leave-active {
  overflow: hidden;
  transition: opacity 180ms ease, transform 180ms ease;
}

.advanced-fields-enter-from,
.advanced-fields-leave-to {
  opacity: 0;
  transform: translateY(-5px);
}

@keyframes estimator-spin {
  to { transform: rotate(360deg); }
}

@keyframes estimator-pulse {
  0%, 100% { opacity: 0.35; }
  50% { opacity: 1; }
}

@media (max-width: 900px) {
  .billing-estimator__tool {
    grid-template-columns: 1fr;
  }

  .billing-estimator__result {
    border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
    border-left: 0;
  }
}

@media (max-width: 620px) {
  .billing-estimator__form,
  .billing-estimator__result {
    padding: 16px;
  }

  .billing-estimator__toolbar {
    align-items: flex-start;
  }

  .reset-button span {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  .billing-estimator__selection,
  .billing-estimator__tokens,
  .billing-estimator__advanced {
    grid-template-columns: 1fr;
  }

  .billing-estimator__rate-strip {
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  }

  .billing-estimator__rate-strip .billing-estimator__rate-operator:nth-of-type(2),
  .billing-estimator__rate-total {
    display: none;
  }

  .billing-estimator__rate-strip em {
    grid-column: 1 / -1;
    width: fit-content;
  }
}

@media (prefers-reduced-motion: reduce) {
  .billing-estimator__spinner,
  .billing-estimator__result-heading i {
    animation: none;
  }

  .contribution-row__bar,
  .billing-estimator__advanced-toggle svg,
  .advanced-fields-enter-active,
  .advanced-fields-leave-active {
    transition: none;
  }
}
</style>
