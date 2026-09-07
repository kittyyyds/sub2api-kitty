<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import { useAuthStore } from '@/stores/auth'
import type { GroupPlatform } from '@/types'
import { platformLabel } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'

import {
  buildCatalogSections,
  groupTextRate,
  visibleCatalogGroups,
  type CatalogKind,
  type CatalogMetric,
} from './modelCatalog'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  embedded?: boolean
}>()

const emit = defineEmits<{
  retry: []
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const { isDarkMode } = useDocumentDarkMode()
const { copyToClipboard } = useClipboard()

const selectedGroupId = shallowRef<number | null>(null)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const groups = computed(() => visibleCatalogGroups(props.response?.groups ?? []))
const sections = computed(() => buildCatalogSections(props.response?.groups ?? [], selectedGroupId.value))
const modelCount = computed(() => sections.value.reduce((total, section) => total + section.cards.length, 0))

const descriptionHtml = computed(() => {
  const markdown = props.response?.description?.trim()
  if (!markdown) return ''
  return DOMPurify.sanitize(marked.parse(markdown) as string)
})

watch(groups, (nextGroups) => {
  if (
    selectedGroupId.value != null
    && !nextGroups.some((group) => group.id === selectedGroupId.value)
  ) {
    selectedGroupId.value = null
  }
})

function selectGroup(groupId: number | null): void {
  selectedGroupId.value = groupId
}

function groupRate(group: ModelPlazaGroup): string {
  return rateLabel(groupTextRate(group))
}

function groupUsesPersonalRate(group: ModelPlazaGroup): boolean {
  return group.user_rate_multiplier != null
    && Math.abs(group.user_rate_multiplier - group.rate_multiplier) > Number.EPSILON
}

function rateLabel(value: number): string {
  return `×${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: value >= 1 ? 2 : 4
  }).format(value)}`
}

function sectionTitle(platform: string, kind: CatalogKind): string {
  return t('modelPlaza.catalog.sectionTitle', {
    platform: platformLabel(platform),
    kind: kindLabel(kind)
  })
}

function kindLabel(kind: CatalogKind): string {
  return t(`modelPlaza.catalog.kinds.${kind}`)
}

function metricTitle(metric: CatalogMetric, kind: CatalogKind): string {
  if (metric.label === 'unit') {
    const unit = kind === 'image'
      ? t('modelPlaza.catalog.units.image')
      : kind === 'video'
        ? t('modelPlaza.catalog.units.second')
        : t('modelPlaza.catalog.units.request')
    return metric.tier ? `${metric.tier} / ${unit}` : unit
  }
  return `${t(`modelPlaza.catalog.metrics.${metric.label}`)} / 1M`
}

function displayPrice(value: number | null, kind: CatalogKind): string {
  return formatScaled(value, kind === 'token' ? 1_000_000 : 1, 2)
}

function savingLabel(value: number): string {
  return `-${new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(value)}%`
}

function copyModelId(modelId: string): void {
  void copyToClipboard(modelId, t('modelPlaza.catalog.copySuccess'))
}
</script>

<template>
  <main class="model-catalog" :class="{ 'is-dark': isDarkMode }">
    <section
      v-if="groups.length > 0"
      class="pricing-group-panel"
      :aria-label="t('modelPlaza.catalog.groupSelector')"
    >
      <button
        type="button"
        class="pricing-group-chip"
        :class="{ active: selectedGroupId === null }"
        :aria-pressed="selectedGroupId === null"
        @click="selectGroup(null)"
      >
        <Icon name="sparkles" size="sm" />
        <span>{{ t('modelPlaza.catalog.bestPrice') }}</span>
      </button>

      <button
        v-for="group in groups"
        :key="group.id"
        type="button"
        class="pricing-group-chip"
        :class="{ active: selectedGroupId === group.id }"
        :aria-pressed="selectedGroupId === group.id"
        :title="group.name"
        @click="selectGroup(group.id)"
      >
        <PlatformIcon :platform="group.platform as GroupPlatform" size="sm" />
        <span class="pricing-group-name">{{ group.name }}</span>
        <small :title="groupUsesPersonalRate(group) ? t('modelPlaza.catalog.personalRate') : undefined">
          {{ groupRate(group) }}
        </small>
      </button>
    </section>

    <div
      v-if="descriptionHtml"
      class="catalog-notice"
      v-html="descriptionHtml"
    ></div>

    <p v-if="!isAuthenticated" class="catalog-hint">
      <Icon name="infoCircle" size="xs" />
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <div v-if="loading" class="state-panel" :aria-label="t('modelPlaza.loading')">
      <div class="state-spinner"></div>
      <p>{{ t('modelPlaza.loading') }}</p>
    </div>

    <div v-else-if="error" class="state-panel error">
      <Icon name="exclamationTriangle" size="lg" />
      <p>{{ t('modelPlaza.loadFailed') }}</p>
      <button type="button" class="retry-button" @click="emit('retry')">
        <Icon name="refresh" size="sm" />
        {{ t('common.refresh') }}
      </button>
    </div>

    <div v-else-if="modelCount === 0" class="state-panel">
      <Icon name="inbox" size="xl" />
      <p>{{ t('modelPlaza.empty') }}</p>
    </div>

    <div v-else class="platform-section-list">
      <section
        v-for="section in sections"
        :key="section.key"
        class="platform-model-section"
      >
        <header class="platform-section-header">
          <div>
            <h2>{{ sectionTitle(section.platform, section.kind) }}</h2>
            <p>{{ t('modelPlaza.catalog.modelCount', { count: section.cards.length }) }}</p>
          </div>
        </header>

        <div class="platform-grid">
          <article
            v-for="card in section.cards"
            :key="card.key"
            class="platform-card"
          >
            <header class="platform-card-header">
              <div class="platform-title-group">
                <div class="platform-model-icon" aria-hidden="true">
                  <ModelIcon :model="card.modelId" size="34px" />
                </div>
                <div class="model-title-copy">
                  <h3 :title="card.modelName">{{ card.modelName }}</h3>
                  <div class="platform-meta-row">
                    <span class="platform-provider-pill">
                      <PlatformIcon :platform="card.platform as GroupPlatform" size="sm" />
                      {{ platformLabel(card.platform) }}
                    </span>
                    <span class="platform-kind-pill">{{ kindLabel(card.kind) }}</span>
                  </div>
                </div>
              </div>

              <div class="platform-header-actions">
                <span class="best-group-pill">
                  {{ t(
                    selectedGroupId === null
                      ? 'modelPlaza.catalog.bestGroup'
                      : 'modelPlaza.catalog.currentGroup',
                    { group: card.groupName }
                  ) }}
                </span>
                <button
                  type="button"
                  class="copy-id-button"
                  :title="t('modelPlaza.catalog.copyModelId')"
                  @click="copyModelId(card.modelId)"
                >
                  <Icon name="copy" size="sm" />
                  <span>{{ t('modelPlaza.catalog.copyId') }}</span>
                </button>
              </div>
            </header>

            <div class="platform-model-id-row">
              <span>{{ t('modelPlaza.catalog.modelId') }}</span>
              <code :title="card.modelId">{{ card.modelId }}</code>
            </div>

            <div
              class="platform-price-grid"
              :class="`price-grid-${Math.min(card.metrics.length, 4)}`"
            >
              <div
                v-for="metric in card.metrics"
                :key="metric.key"
                class="price-metric"
              >
                <p class="price-metric-title">{{ metricTitle(metric, card.kind) }}</p>
                <div class="price-metric-body">
                  <div class="price-line">
                    <span>{{ t('modelPlaza.catalog.currentPrice') }}</span>
                    <strong>{{ displayPrice(metric.current, card.kind) }}</strong>
                  </div>
                  <div v-if="metric.official != null" class="price-line official">
                    <span>{{ t('modelPlaza.catalog.officialPrice') }}</span>
                    <span>{{ displayPrice(metric.official, card.kind) }}</span>
                  </div>
                </div>
                <div v-if="metric.saving != null" class="price-saving-row">
                  <span>{{ t('modelPlaza.catalog.saving') }}</span>
                  <strong>{{ savingLabel(metric.saving) }}</strong>
                </div>
              </div>
            </div>

            <div class="platform-footer-row">
              <span>{{ t('modelPlaza.catalog.effectiveMultiplier') }}</span>
              <strong>{{ rateLabel(card.multiplier) }}</strong>
            </div>

            <div
              v-if="card.hasLongContextPricing || card.hasTimePricing"
              class="platform-note-list"
            >
              <span v-if="card.hasLongContextPricing" class="platform-note-chip">
                {{ t('modelPlaza.catalog.longContextNote') }}
              </span>
              <span v-if="card.hasTimePricing" class="platform-note-chip">
                {{ t('modelPlaza.catalog.timePricingNote') }}
              </span>
            </div>
          </article>
        </div>
      </section>
    </div>
  </main>
</template>

<style scoped>
.model-catalog {
  --catalog-surface: #ffffff;
  --catalog-surface-muted: #f7f9fb;
  --catalog-surface-soft: #eef3f5;
  --catalog-line: #dfe6eb;
  --catalog-line-strong: #cdd8df;
  --catalog-text: #526171;
  --catalog-text-strong: #172330;
  --catalog-muted: #748191;
  --catalog-accent: #0f766e;
  --catalog-accent-strong: #0b625c;
  --catalog-accent-soft: #eaf8f5;
  --catalog-active-text: #ffffff;
  --catalog-saving: #c25716;
  --catalog-saving-bg: #fff4ea;
  --catalog-saving-line: #f1c9aa;
  --catalog-danger: #dc2626;
  width: 100%;
  max-width: 1280px;
  margin-inline: auto;
  color: var(--catalog-text);
}

.model-catalog.is-dark {
  --catalog-surface: #111b29;
  --catalog-surface-muted: #0d1724;
  --catalog-surface-soft: #182636;
  --catalog-line: #29394a;
  --catalog-line-strong: #3a4b5e;
  --catalog-text: #c6d0db;
  --catalog-text-strong: #f5f7fa;
  --catalog-muted: #96a3b3;
  --catalog-accent: #4dd8c5;
  --catalog-accent-strong: #61e7d4;
  --catalog-accent-soft: rgb(45 212 191 / 11%);
  --catalog-active-text: #071514;
  --catalog-saving: #fdba74;
  --catalog-saving-bg: rgb(194 65 12 / 16%);
  --catalog-saving-line: rgb(251 146 60 / 28%);
  --catalog-danger: #fca5a5;
}

.pricing-group-panel {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--catalog-line);
  border-radius: 8px;
  padding: 12px;
  background: var(--catalog-surface);
  box-shadow: 0 1px 2px rgb(15 23 42 / 5%);
}

.pricing-group-chip {
  display: inline-flex;
  height: 36px;
  max-width: 100%;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--catalog-line);
  border-radius: 6px;
  padding: 0 12px;
  color: var(--catalog-text);
  background: var(--catalog-surface-muted);
  font-size: 13px;
  font-weight: 600;
  transition: border-color 150ms ease, color 150ms ease, background 150ms ease;
}

.pricing-group-chip:hover {
  border-color: color-mix(in srgb, var(--catalog-accent) 62%, var(--catalog-line));
  color: var(--catalog-accent-strong);
  background: var(--catalog-accent-soft);
}

.pricing-group-chip.active {
  border-color: var(--catalog-accent);
  color: var(--catalog-active-text);
  background: var(--catalog-accent);
}

.pricing-group-chip small {
  flex: 0 0 auto;
  color: inherit;
  opacity: .8;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
}

.pricing-group-name {
  max-width: 176px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.catalog-notice,
.catalog-hint {
  margin: 12px 0 0;
  border: 1px solid var(--catalog-line);
  border-radius: 6px;
  padding: 10px 12px;
  color: var(--catalog-muted);
  background: var(--catalog-surface-muted);
  font-size: 12px;
  line-height: 1.65;
}

.catalog-hint {
  display: flex;
  align-items: center;
  gap: 6px;
}

.catalog-notice :deep(p) {
  margin: 0;
}

.catalog-notice :deep(a) {
  color: var(--catalog-accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.state-panel {
  display: flex;
  min-height: 288px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-top: 16px;
  border: 1px dashed var(--catalog-line-strong);
  border-radius: 8px;
  padding: 32px;
  color: var(--catalog-muted);
  background: var(--catalog-surface);
  text-align: center;
  font-size: 13px;
}

.state-panel p {
  margin: 0;
}

.state-panel.error {
  color: var(--catalog-danger);
}

.state-spinner {
  width: 32px;
  height: 32px;
  animation: spin 1s linear infinite;
  border: 3px solid color-mix(in srgb, var(--catalog-accent) 24%, transparent);
  border-top-color: var(--catalog-accent);
  border-radius: 50%;
}

.retry-button,
.copy-id-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid var(--catalog-line);
  border-radius: 6px;
  color: var(--catalog-text);
  background: var(--catalog-surface);
  font-weight: 600;
}

.retry-button {
  min-height: 34px;
  padding: 0 12px;
  font-size: 12px;
}

.platform-section-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
  margin-top: 18px;
}

.platform-model-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.platform-section-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
}

.platform-section-header h2 {
  margin: 0;
  color: var(--catalog-text-strong);
  font-size: 19px;
  font-weight: 700;
}

.platform-section-header p {
  margin: 3px 0 0;
  color: var(--catalog-muted);
  font-size: 12px;
}

.platform-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 14px;
}

.platform-card {
  min-width: 0;
  border: 1px solid var(--catalog-line);
  border-radius: 8px;
  padding: 14px;
  background: var(--catalog-surface);
  box-shadow: 0 1px 2px rgb(15 23 42 / 5%);
}

.platform-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.platform-title-group {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 12px;
}

.platform-model-icon {
  display: flex;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--catalog-line);
  border-radius: 8px;
  background: var(--catalog-surface);
}

.model-title-copy {
  min-width: 0;
}

.platform-card h3 {
  margin: 0;
  overflow: hidden;
  color: var(--catalog-text-strong);
  font-size: 17px;
  font-weight: 700;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.platform-meta-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 5px;
}

.platform-provider-pill,
.platform-kind-pill,
.best-group-pill,
.platform-note-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 999px;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
}

.platform-provider-pill,
.best-group-pill,
.platform-note-chip {
  color: var(--catalog-text);
  background: var(--catalog-surface-soft);
}

.platform-kind-pill {
  color: #0369a1;
  background: #eff8ff;
}

.is-dark .platform-kind-pill {
  color: #7dd3fc;
  background: rgb(14 165 233 / 14%);
}

.platform-header-actions {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}

.best-group-pill {
  max-width: 190px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copy-id-button {
  height: 32px;
  padding: 0 9px;
  color: var(--catalog-muted);
  font-size: 12px;
}

.copy-id-button:hover,
.retry-button:hover {
  border-color: color-mix(in srgb, var(--catalog-accent) 62%, var(--catalog-line));
  color: var(--catalog-accent-strong);
  background: var(--catalog-accent-soft);
}

.platform-model-id-row {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  border-radius: 6px;
  padding: 8px 10px;
  background: var(--catalog-surface-muted);
  font-size: 11px;
}

.platform-model-id-row span {
  flex: 0 0 auto;
  color: var(--catalog-muted);
  font-weight: 600;
}

.platform-model-id-row code {
  overflow: hidden;
  color: var(--catalog-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.platform-price-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  margin-top: 12px;
  overflow: hidden;
  border: 1px solid var(--catalog-line);
  border-radius: 6px;
  background: var(--catalog-surface-muted);
}

.price-metric {
  min-width: 0;
  padding: 9px;
  background: var(--catalog-surface-muted);
}

.price-metric + .price-metric {
  border-top: 1px solid var(--catalog-line);
}

.price-metric-title {
  margin: 0;
  color: var(--catalog-muted);
  font-size: 12px;
  font-weight: 600;
}

.price-metric-body {
  display: grid;
  gap: 3px;
  margin-top: 6px;
}

.price-line {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.price-line > span:first-child {
  color: var(--catalog-muted);
  font-size: 10px;
}

.price-line strong {
  color: var(--catalog-text-strong);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 14px;
  font-weight: 750;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.price-line.official > span:last-child {
  color: var(--catalog-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.price-saving-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 7px;
  border: 1px solid var(--catalog-saving-line);
  border-radius: 5px;
  padding: 3px 6px;
  color: var(--catalog-saving);
  background: var(--catalog-saving-bg);
  font-size: 10px;
  font-weight: 700;
}

.platform-footer-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 12px;
  border-top: 1px solid var(--catalog-line);
  padding-top: 10px;
  color: var(--catalog-muted);
  font-size: 12px;
}

.platform-footer-row strong {
  color: var(--catalog-text-strong);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 14px;
}

.platform-note-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.platform-note-chip {
  color: var(--catalog-muted);
  font-size: 10px;
}

@media (min-width: 640px) {
  .platform-price-grid.price-grid-2,
  .platform-price-grid.price-grid-3,
  .platform-price-grid.price-grid-4 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .price-metric + .price-metric {
    border-top: 0;
    border-left: 1px solid var(--catalog-line);
  }

  .price-grid-3 .price-metric:nth-child(3) {
    border-top: 1px solid var(--catalog-line);
    border-left: 0;
  }

  .price-grid-4 .price-metric:nth-child(3) {
    border-top: 1px solid var(--catalog-line);
    border-left: 0;
  }

  .price-grid-4 .price-metric:nth-child(4) {
    border-top: 1px solid var(--catalog-line);
  }
}

@media (min-width: 980px) {
  .platform-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1440px) {
  .platform-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 560px) {
  .pricing-group-panel {
    flex-wrap: nowrap;
    overflow-x: auto;
  }

  .platform-card-header {
    display: block;
  }

  .platform-header-actions {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    margin-top: 10px;
  }

  .best-group-pill {
    max-width: calc(100% - 92px);
  }

  .copy-id-button span {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .state-spinner {
    animation-duration: 2s;
  }

  .pricing-group-chip {
    transition: none;
  }
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
