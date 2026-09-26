<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import AIRecommendationPanel from '@/components/customer-console/ai-intelligence/AIRecommendationPanel.vue'
import AIRecommendationTable from '@/components/customer-console/ai-intelligence/AIRecommendationTable.vue'
import AIRankingList from '@/components/customer-console/ai-intelligence/AIRankingList.vue'
import AIServiceStatus from '@/components/customer-console/ai-intelligence/AIServiceStatus.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAIIntelligence } from '@/composables/useAIIntelligence'
import { buildAIIntelligenceView } from '@/features/customer-console/aiIntelligence'

const { t, locale } = useI18n()
const { payload, loading, error, stale, refresh } = useAIIntelligence()

const view = computed(() => buildAIIntelligenceView(payload.value?.points ?? []))
const recommendationBundle = computed(() => {
  const { recommended, smartest, bestValue, fastest } = view.value
  if (!recommended || !smartest || !bestValue || !fastest) return null
  return { recommended, smartest, bestValue, fastest }
})

const updatedLabel = computed(() => {
  const value = payload.value?.source_updated_at || payload.value?.fetched_at
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return value
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
})
</script>

<template>
  <section class="ai-intelligence-page" data-native-feature="reset-radar">
    <AIServiceStatus v-if="payload" :status="payload.service_status" />

    <div v-if="payload && (stale || loading)" class="sync-banner" :data-stale="stale || undefined">
      <Icon :name="stale ? 'exclamationTriangle' : 'refresh'" size="sm" />
      <span>
        {{ stale ? t('native.radar.sync.stale') : t('native.radar.sync.refreshing') }}
        <small v-if="updatedLabel">{{ t('native.radar.sync.updated', { time: updatedLabel }) }}</small>
      </span>
      <button
        type="button"
        :disabled="loading"
        :title="t('native.radar.sync.refresh')"
        :aria-label="t('native.radar.sync.refresh')"
        @click="refresh({ force: true })"
      >
        <Icon name="refresh" size="sm" :class="{ 'is-spinning': loading }" />
      </button>
    </div>

    <div v-if="loading && !payload" class="intelligence-loading" :aria-label="t('native.radar.loading')">
      <span class="loading-status"></span>
      <div class="loading-grid">
        <span></span><span></span><span></span>
      </div>
    </div>

    <div v-else-if="!payload" class="intelligence-empty">
      <span aria-hidden="true"><Icon name="exclamationTriangle" size="lg" /></span>
      <h1>{{ t('native.radar.error.title') }}</h1>
      <p>{{ error || t('native.radar.error.description') }}</p>
      <button type="button" :disabled="loading" @click="refresh({ force: true })">
        <Icon name="refresh" size="sm" />
        {{ t('native.radar.error.retry') }}
      </button>
    </div>

    <template v-else-if="recommendationBundle">
      <div class="intelligence-grid">
        <main class="analysis-column">
          <AIRecommendationPanel
            :recommended="recommendationBundle.recommended"
            :smartest="recommendationBundle.smartest"
            :best-value="recommendationBundle.bestValue"
            :fastest="recommendationBundle.fastest"
          />
          <AIRecommendationTable :rows="view.recommendations" />
        </main>

        <AIRankingList kind="intelligence" :items="view.intelligenceRanking" />
        <AIRankingList kind="speed" :items="view.speedRanking" />
      </div>

      <footer class="data-attribution">
        <Icon name="infoCircle" size="sm" />
        <p>{{ t('native.radar.attribution') }}</p>
        <div>
          <a href="https://codexradar.com/" target="_blank" rel="noopener noreferrer">Codex Radar</a>
          <a href="https://status.openai.com/" target="_blank" rel="noopener noreferrer">OpenAI Status</a>
        </div>
      </footer>
    </template>

    <div v-else class="intelligence-empty is-compact">
      <span aria-hidden="true"><Icon name="brain" size="lg" /></span>
      <h1>{{ t('native.radar.error.noPointsTitle') }}</h1>
      <p>{{ t('native.radar.error.noPointsDescription') }}</p>
    </div>
  </section>
</template>

<style scoped>
.ai-intelligence-page {
  --ai-surface: #ffffff;
  --ai-surface-soft: #f4f7fa;
  --ai-ink: #17212b;
  --ai-muted: #647181;
  --ai-faint: #8995a3;
  --ai-line: #d7dee6;
  --ai-line-strong: #aab6c3;
  --ai-track: #e3e8ee;
  --ai-cyan: #0f766e;
  --ai-cyan-soft: #e7f7f4;
  --ai-blue: #2563eb;
  --ai-blue-soft: #eaf1ff;
  --ai-violet: #7c3aed;
  --ai-violet-soft: #f2ebff;
  --ai-copper: #c2410c;
  --ai-copper-soft: #fff1e7;
  --ai-success: #169b55;
  --ai-success-soft: #e9f8ef;
  --ai-warning: #d79100;
  --ai-warning-soft: #fff7dc;
  --ai-danger: #dc3545;
  --ai-danger-soft: #fff0f1;
  --ai-gold: #e9a916;
  --ai-gold-soft: #fff7dd;
  --ai-silver: #94a3b8;
  --ai-bronze: #c67536;
  --ai-mono: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  --ai-shadow: 0 12px 30px rgb(28 39 51 / 5%);

  display: grid;
  gap: 18px;
  min-width: 0;
  color: var(--ai-ink);
  font-family: Inter, "Noto Sans SC", "Microsoft YaHei", sans-serif;
}

:global(.dark) .ai-intelligence-page {
  --ai-surface: #111827;
  --ai-surface-soft: #1b2637;
  --ai-ink: #f3f6fa;
  --ai-muted: #a9b4c2;
  --ai-faint: #7f8b9a;
  --ai-line: #354155;
  --ai-line-strong: #59677a;
  --ai-track: #2a3445;
  --ai-cyan: #2dd4bf;
  --ai-cyan-soft: rgb(45 212 191 / 13%);
  --ai-blue: #60a5fa;
  --ai-blue-soft: rgb(96 165 250 / 13%);
  --ai-violet: #c4a1ff;
  --ai-violet-soft: rgb(196 161 255 / 13%);
  --ai-copper: #fb923c;
  --ai-copper-soft: rgb(251 146 60 / 13%);
  --ai-success: #35c777;
  --ai-success-soft: rgb(53 199 119 / 12%);
  --ai-warning: #f0b429;
  --ai-warning-soft: rgb(240 180 41 / 12%);
  --ai-danger: #fb7185;
  --ai-danger-soft: rgb(251 113 133 / 12%);
  --ai-gold: #e9a916;
  --ai-gold-soft: rgb(233 169 22 / 11%);
  --ai-silver: #94a3b8;
  --ai-bronze: #d48649;
  --ai-shadow: 0 14px 34px rgb(0 0 0 / 18%);
}

.sync-banner {
  display: flex;
  min-height: 40px;
  align-items: center;
  gap: 9px;
  padding: 7px 10px 7px 12px;
  border: 1px solid var(--ai-line);
  border-radius: 7px;
  color: var(--ai-muted);
  background: var(--ai-surface-soft);
  font-size: 10px;
}

.sync-banner[data-stale="true"] {
  border-color: color-mix(in srgb, var(--ai-warning) 38%, var(--ai-line));
  color: var(--ai-warning);
  background: var(--ai-warning-soft);
}

.sync-banner > span {
  display: flex;
  flex-wrap: wrap;
  gap: 5px 8px;
}

.sync-banner small {
  color: var(--ai-faint);
  font-size: 9px;
}

.sync-banner button {
  display: grid;
  width: 30px;
  height: 30px;
  margin-left: auto;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid var(--ai-line);
  border-radius: 6px;
  color: var(--ai-muted);
  background: var(--ai-surface);
}

.sync-banner button:hover:not(:disabled) {
  border-color: var(--ai-cyan);
  color: var(--ai-cyan);
}

.sync-banner button:focus-visible,
.intelligence-empty button:focus-visible {
  outline: 2px solid var(--ai-cyan);
  outline-offset: 2px;
}

.sync-banner button:disabled {
  opacity: 0.5;
}

.is-spinning {
  animation: ai-spin 800ms linear infinite;
}

.intelligence-grid {
  display: grid;
  grid-template-columns: minmax(680px, 1fr) minmax(238px, 278px) minmax(238px, 278px);
  align-items: start;
  gap: 18px;
  min-width: 0;
}

.analysis-column {
  display: grid;
  min-width: 0;
  gap: 18px;
}

.data-attribution {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  min-width: 0;
  padding: 10px 12px;
  border-left: 2px solid var(--ai-cyan);
  color: var(--ai-muted);
  background: var(--ai-cyan-soft);
  font-size: 10px;
}

.data-attribution p {
  min-width: 0;
  margin: 0;
  line-height: 1.55;
}

.data-attribution div {
  display: flex;
  gap: 12px;
  margin-left: auto;
  white-space: nowrap;
}

.data-attribution a {
  color: var(--ai-cyan);
  font-weight: 650;
}

.data-attribution a:hover {
  text-decoration: underline;
}

.intelligence-loading {
  display: grid;
  gap: 18px;
  min-height: 560px;
}

.loading-status,
.loading-grid span {
  border: 1px solid var(--ai-line);
  border-radius: 8px;
  background: var(--ai-surface-soft);
  animation: ai-pulse 1.1s ease-in-out infinite alternate;
}

.loading-status {
  height: 92px;
}

.loading-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 278px 278px;
  gap: 18px;
}

.loading-grid span {
  min-height: 450px;
}

.intelligence-empty {
  display: grid;
  min-height: 520px;
  place-items: center;
  align-content: center;
  gap: 9px;
  padding: 24px;
  border: 1px solid var(--ai-line);
  border-radius: 8px;
  color: var(--ai-muted);
  background: var(--ai-surface);
  text-align: center;
}

.intelligence-empty.is-compact {
  min-height: 280px;
}

.intelligence-empty > span {
  display: grid;
  width: 44px;
  height: 44px;
  place-items: center;
  border-radius: 7px;
  color: var(--ai-warning);
  background: var(--ai-warning-soft);
}

.intelligence-empty h1 {
  margin: 5px 0 0;
  color: var(--ai-ink);
  font-size: 16px;
  letter-spacing: 0;
}

.intelligence-empty p {
  max-width: 460px;
  margin: 0;
  font-size: 11px;
}

.intelligence-empty button {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  gap: 7px;
  margin-top: 7px;
  padding: 0 12px;
  border: 1px solid var(--ai-line-strong);
  border-radius: 6px;
  color: var(--ai-cyan);
  background: var(--ai-cyan-soft);
  font-size: 10px;
  font-weight: 650;
}

@keyframes ai-spin {
  to { transform: rotate(360deg); }
}

@keyframes ai-pulse {
  to { opacity: 0.5; }
}

@media (max-width: 1380px) {
  .intelligence-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .analysis-column {
    grid-column: 1 / -1;
  }
}

@media (max-width: 760px) {
  .ai-intelligence-page {
    gap: 14px;
  }

  .intelligence-grid,
  .loading-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 14px;
  }

  .analysis-column {
    grid-column: auto;
    gap: 14px;
  }

  .data-attribution {
    flex-wrap: wrap;
  }

  .data-attribution div {
    width: 100%;
    margin-left: 24px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .is-spinning,
  .loading-status,
  .loading-grid span {
    animation: none;
  }
}
</style>
