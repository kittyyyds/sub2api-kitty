<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'
import Icon from '@/components/icons/Icon.vue'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import { useAuthStore } from '@/stores/auth'

import PlazaFilterBar from './PlazaFilterBar.vue'
import PlazaGroupSection from './PlazaGroupSection.vue'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  /** AppLayout already supplies the route title when this page is embedded. */
  embedded?: boolean
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const { isDarkMode } = useDocumentDarkMode()

const selectedPlatform = shallowRef('')
const selectedGroupId = shallowRef<number | null>(null)
const searchQuery = shallowRef('')

const isAuthenticated = computed(() => authStore.isAuthenticated)
const searchActive = computed(() => searchQuery.value.trim() !== '')

const descriptionHtml = computed(() => {
  const markdown = props.response?.description?.trim()
  if (!markdown) return ''
  return DOMPurify.sanitize(marked.parse(markdown) as string)
})

function effectiveRate(group: ModelPlazaGroup): number {
  return group.user_rate_multiplier ?? group.rate_multiplier
}

const platforms = computed(() => {
  const values = [...new Set(
    (props.response?.groups ?? []).map((group) => group.platform).filter(Boolean)
  )]
  const priority = ['openai', 'anthropic', 'grok']
  return values.sort((a, b) => {
    const aIndex = priority.indexOf(a)
    const bIndex = priority.indexOf(b)
    if (aIndex !== -1 || bIndex !== -1) {
      return (aIndex === -1 ? priority.length : aIndex) - (bIndex === -1 ? priority.length : bIndex)
    }
    return a.localeCompare(b)
  })
})

const groupOptions = computed(() =>
  (props.response?.groups ?? [])
    .map((group) => ({
      id: group.id,
      name: group.name,
      platform: group.platform,
      rate: effectiveRate(group),
      usesPersonalRate: group.user_rate_multiplier != null
        && Math.abs(group.user_rate_multiplier - group.rate_multiplier) > Number.EPSILON
    }))
    .sort((a, b) => a.rate - b.rate || a.name.localeCompare(b.name))
)

const visibleGroupOptions = computed(() =>
  groupOptions.value.filter((group) => group.platform === selectedPlatform.value)
)

watch(groupOptions, (groups) => {
  if (groups.length === 0) {
    selectedPlatform.value = ''
    selectedGroupId.value = null
    return
  }

  if (!platforms.value.includes(selectedPlatform.value)) {
    selectedPlatform.value = platforms.value[0] ?? groups[0].platform
  }

  const availableGroups = groups.filter((group) => group.platform === selectedPlatform.value)
  if (!availableGroups.some((group) => group.id === selectedGroupId.value)) {
    selectedGroupId.value = availableGroups[0]?.id ?? null
  }
}, { immediate: true })

function selectPlatform(platform: string): void {
  selectedPlatform.value = platform
  selectedGroupId.value = groupOptions.value.find((group) => group.platform === platform)?.id ?? null
}

function selectGroup(groupId: number): void {
  const group = groupOptions.value.find((item) => item.id === groupId)
  if (!group) return
  selectedPlatform.value = group.platform
  selectedGroupId.value = group.id
}

const currentGroup = computed<ModelPlazaGroup | null>(() => {
  const group = (props.response?.groups ?? []).find((item) => item.id === selectedGroupId.value)
  if (!group) return null

  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return group

  const models = group.models.filter((model) => model.name.toLowerCase().includes(query))
  return models.length > 0 ? { ...group, models } : null
})
</script>

<template>
  <main class="model-plaza-content" :class="{ 'is-dark': isDarkMode }">
    <section class="pricing-console" aria-labelledby="model-pricing-title">
      <header class="pricing-intro">
        <h1 id="model-pricing-title">{{ t('modelPlaza.title') }}</h1>
        <p>{{ t('modelPlaza.description') }}</p>
        <div class="pricing-facts">
          <span class="exchange-rate">$1 = ¥7</span>
          <span class="billing-rule">
            <Icon name="infoCircle" size="xs" />
            {{ t('modelPlaza.billingRule') }}
          </span>
        </div>
      </header>

      <div
        v-if="descriptionHtml"
        class="plaza-description"
        v-html="descriptionHtml"
      ></div>

      <p v-if="!isAuthenticated" class="anonymous-hint">
        <Icon name="infoCircle" size="xs" />
        {{ t('modelPlaza.anonymousHint') }}
      </p>

      <div v-if="loading" class="loading-state" :aria-label="t('modelPlaza.loading')">
        <div class="loading-spinner"></div>
      </div>
      <div v-else-if="error" class="error-state">
        {{ t('modelPlaza.loadFailed') }}
      </div>
      <template v-else>
        <PlazaFilterBar
          :platforms="platforms"
          :groups="visibleGroupOptions"
          :platform="selectedPlatform"
          :group-id="selectedGroupId"
          :search="searchQuery"
          @update:platform="selectPlatform"
          @update:group-id="selectGroup"
          @update:search="searchQuery = $event"
        />

        <Transition name="group-price" mode="out-in">
          <PlazaGroupSection
            v-if="currentGroup"
            :key="currentGroup.id"
            :group="currentGroup"
          />
          <div v-else :key="'empty'" class="empty-state">
            {{ searchActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}
          </div>
        </Transition>
      </template>
    </section>
  </main>
</template>

<style scoped>
.model-plaza-content {
  --plaza-surface: #ffffff;
  --plaza-surface-muted: #f7f9fb;
  --plaza-surface-soft: #f1f5f7;
  --plaza-line: #dce4ea;
  --plaza-line-soft: #e9eef2;
  --plaza-line-strong: #cbd6de;
  --plaza-text: #526171;
  --plaza-text-strong: #172330;
  --plaza-muted: #748191;
  --plaza-muted-soft: #8793a1;
  --plaza-accent: #0f766e;
  --plaza-accent-soft: #ecfdf8;
  --plaza-accent-ring: rgb(15 118 110 / 12%);
  --plaza-active-mix: #0f172a;
  --plaza-hover: #f8fbfa;
  --plaza-reference: #8b98a7;
  --plaza-reference-strike: #a6b0bb;
  --plaza-price: #c2710c;
  --plaza-success: #047857;
  --plaza-success-bg: #dcfce7;
  --plaza-subscription: #6d28d9;
  --plaza-peak-border: #f5d99a;
  --plaza-peak-bg: #fff9e9;
  --plaza-peak-text: #9a5b0a;
  width: 100%;
  max-width: 1120px;
  margin-inline: auto;
  color: var(--plaza-text);
}

.model-plaza-content.is-dark {
  --plaza-surface: #0f1928;
  --plaza-surface-muted: #111d2c;
  --plaza-surface-soft: #162334;
  --plaza-line: #29384a;
  --plaza-line-soft: #223043;
  --plaza-line-strong: #35465a;
  --plaza-text: #c3cbd5;
  --plaza-text-strong: #f1f5f9;
  --plaza-muted: #95a2b3;
  --plaza-muted-soft: #8290a2;
  --plaza-accent: #4de2ce;
  --plaza-accent-soft: rgb(45 212 191 / 10%);
  --plaza-accent-ring: rgb(45 212 191 / 15%);
  --plaza-active-mix: #07111f;
  --plaza-hover: rgb(45 212 191 / 4%);
  --plaza-reference: #8492a4;
  --plaza-reference-strike: #64748b;
  --plaza-price: #fbbf24;
  --plaza-success: #5ee7ba;
  --plaza-success-bg: rgb(16 185 129 / 14%);
  --plaza-subscription: #c4b5fd;
  --plaza-peak-border: rgb(180 83 9 / 45%);
  --plaza-peak-bg: rgb(120 53 15 / 16%);
  --plaza-peak-text: #fcd34d;
}

.pricing-console {
  overflow: hidden;
  border: 1px solid var(--plaza-line);
  border-radius: 8px;
  background: var(--plaza-surface);
}

.pricing-intro {
  display: grid;
  justify-items: center;
  padding: 30px 24px 22px;
  border-bottom: 1px solid var(--plaza-line-soft);
  text-align: center;
}

.pricing-intro h1 {
  margin: 0;
  color: var(--plaza-accent);
  font-size: 22px;
  font-weight: 800;
}

.pricing-intro p {
  max-width: 680px;
  margin: 7px 0 0;
  color: var(--plaza-muted);
  font-size: 12px;
  line-height: 1.6;
}

.pricing-facts {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 14px;
}

.pricing-facts span {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 5px;
  border: 1px solid var(--plaza-line);
  border-radius: 5px;
  padding: 0 10px;
  background: var(--plaza-surface-muted);
  color: var(--plaza-muted);
  font-size: 10px;
}

.pricing-facts .exchange-rate {
  color: var(--plaza-text-strong);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 700;
}

.pricing-facts .billing-rule {
  border-color: color-mix(in srgb, var(--plaza-accent) 28%, var(--plaza-line));
  color: var(--plaza-accent);
  background: var(--plaza-accent-soft);
}

.plaza-description,
.anonymous-hint {
  padding: 11px 18px;
  border-bottom: 1px solid var(--plaza-line-soft);
  background: var(--plaza-surface-muted);
  font-size: 11px;
  line-height: 1.65;
}

.anonymous-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--plaza-muted);
}

.plaza-description {
  overflow-wrap: anywhere;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  margin: 10px 0 5px;
  color: var(--plaza-text-strong);
  font-weight: 700;
}

.plaza-description :deep(h1:first-child),
.plaza-description :deep(h2:first-child),
.plaza-description :deep(h3:first-child) {
  margin-top: 0;
}

.plaza-description :deep(p) {
  margin: 0 0 5px;
}

.plaza-description :deep(p:last-child) {
  margin-bottom: 0;
}

.plaza-description :deep(a) {
  color: var(--plaza-accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}

.plaza-description :deep(ul),
.plaza-description :deep(ol) {
  margin: 0 0 5px;
  padding-left: 18px;
}

.plaza-description :deep(code) {
  border-radius: 3px;
  padding: 1px 4px;
  background: var(--plaza-surface-soft);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.loading-state,
.error-state,
.empty-state {
  display: grid;
  min-height: 240px;
  place-items: center;
  padding: 30px;
  color: var(--plaza-muted);
  font-size: 12px;
}

.loading-spinner {
  width: 30px;
  height: 30px;
  animation: spin 750ms linear infinite;
  border: 2px solid color-mix(in srgb, var(--plaza-accent) 22%, transparent);
  border-top-color: var(--plaza-accent);
  border-radius: 50%;
}

.error-state {
  color: #dc2626;
  background: rgb(239 68 68 / 6%);
}

.group-price-enter-active,
.group-price-leave-active {
  transition: opacity 150ms ease, transform 150ms ease;
}

.group-price-enter-from,
.group-price-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 640px) {
  .pricing-intro {
    padding: 24px 14px 18px;
  }

  .pricing-intro h1 {
    font-size: 20px;
  }

  .pricing-facts {
    width: 100%;
  }

  .pricing-facts span {
    max-width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .loading-spinner {
    animation-duration: 1500ms;
  }

  .group-price-enter-active,
  .group-price-leave-active {
    transition: none;
  }
}
</style>
