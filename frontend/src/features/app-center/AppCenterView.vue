<script setup lang="ts">
import { computed, onMounted, shallowRef, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

import { APP_PRESENTATIONS } from './appRegistry'
import AppCatalogCard from './components/AppCatalogCard.vue'
import AppKeySelectors from './components/AppKeySelectors.vue'
import AppResultPanel from './components/AppResultPanel.vue'
import AppRunHistory from './components/AppRunHistory.vue'
import AppRunStatus from './components/AppRunStatus.vue'
import FreeMediaStudioPanel from './components/media/FreeMediaStudioPanel.vue'
import type { EmbeddedMediaRuntime } from './components/media/FreeMediaStudioPanel.vue'
import EcommerceVisualRunner from './components/runners/EcommerceVisualRunner.vue'
import GrokVideoDirectorRunner from './components/runners/GrokVideoDirectorRunner.vue'
import InspirationImageRunner from './components/runners/InspirationImageRunner.vue'
import { useAppCenter } from './composables/useAppCenter'
import type { ApplicationSlug, RunnerInputPayload } from './types'

const {
  applications,
  history,
  selectedSlug,
  usePlanner,
  selectedApplication,
  selectedPresentation,
  selectedPlannerKeyId,
  selectedMediaKeyId,
  plannerKeyOptions,
  mediaKeyOptions,
  selectedMediaPlatform,
  activeRun,
  selectedHistoryRun,
  historyVideoPreviewUrl,
  historyVideoPreviewLoading,
  historyVideoPreviewError,
  localResult,
  progressSteps,
  isLoading,
  isLoadingHistory,
  isLoadingKeys,
  isRunning,
  error,
  successMessage,
  loadInitial,
  loadApplications,
  loadKeys,
  loadHistory,
  selectApplication,
  runApplication,
  cancelRun,
  stopRun,
  clearCurrentResult,
  resumeActiveRun,
  resumeRun,
  openHistoryRun,
} = useAppCenter()

const route = useRoute()
const router = useRouter()

const query = shallowRef('')
const category = shallowRef<'all' | 'media' | 'image' | 'commerce' | 'video'>('all')

const runnerComponents: Partial<Record<ApplicationSlug, Component>> = {
  'inspiration-image-refiner': InspirationImageRunner,
  'ecommerce-visual-planner': EcommerceVisualRunner,
  'grok-video-director': GrokVideoDirectorRunner,
}

const isFreeMediaStudio = computed(() => selectedSlug.value === 'free-media-studio')
const runnerComponent = computed(() => runnerComponents[selectedSlug.value] || null)
const filteredApplications = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return applications.value.filter((application) => {
    const presentation = APP_PRESENTATIONS[application.slug as ApplicationSlug]
    if (!presentation) return false
    if (category.value !== 'all' && presentation.category !== category.value) return false
    if (!needle) return true
    return `${application.name} ${application.description} ${application.slug}`.toLowerCase().includes(needle)
  })
})

async function refreshWorkspace(): Promise<void> {
  await Promise.all([loadApplications(), loadHistory()])
}

function handleRun(payload: RunnerInputPayload): void {
  void runApplication(payload)
}

const freeMediaRuntime: EmbeddedMediaRuntime = {
  keyOptions: mediaKeyOptions,
  selectedKeyId: selectedMediaKeyId,
  mediaPlatform: selectedMediaPlatform,
  loadingKeys: isLoadingKeys,
  isRunning,
  activeRun,
  localResult,
  error,
  successMessage,
  setSelectedKeyId: (value) => { selectedMediaKeyId.value = value },
  run: handleRun,
  resume: () => { void resumeActiveRun() },
  stop: () => { void stopRun() },
  refreshKeys: () => { void loadKeys() },
  clear: clearCurrentResult,
}

function routeApplicationSlug(value: unknown): ApplicationSlug | null {
  const slug = Array.isArray(value) ? value[0] : value
  return typeof slug === 'string' && slug in APP_PRESENTATIONS ? slug as ApplicationSlug : null
}

function handleSelectApplication(slug: string): void {
  selectApplication(slug)
  void router.replace({ query: { ...route.query, app: slug } })
}

watch(() => route.query.app, (value) => {
  const slug = routeApplicationSlug(value)
  if (slug && slug !== selectedSlug.value) selectApplication(slug)
})

onMounted(() => {
  const slug = routeApplicationSlug(route.query.app)
  if (slug) selectApplication(slug)
  void loadInitial()
})
</script>

<template>
  <AppLayout>
    <section class="app-center" aria-labelledby="app-center-title">
      <header class="app-center-header">
        <div>
          <span class="header-kicker">创作与工作流</span>
          <h1 id="app-center-title">Kitty生图</h1>
        </div>
        <div class="catalog-tools">
          <label class="search-control">
            <Icon name="search" size="sm" />
            <input v-model="query" type="search" placeholder="搜索应用名称或用途" aria-label="搜索应用">
          </label>
          <select v-model="category" aria-label="应用类型">
            <option value="all">全部类型</option>
            <option value="media">自由创作</option>
            <option value="image">图像</option>
            <option value="commerce">电商</option>
            <option value="video">视频</option>
          </select>
          <button type="button" :disabled="isLoading || isLoadingHistory" title="刷新应用与历史" aria-label="刷新应用与历史" @click="refreshWorkspace">
            <Icon name="refresh" size="sm" :class="{ spinning: isLoading || isLoadingHistory }" />
          </button>
        </div>
      </header>

      <div v-if="error || successMessage" class="workspace-message" :class="{ error: !!error }" :role="error ? 'alert' : 'status'">
        <Icon :name="error ? 'exclamationCircle' : 'checkCircle'" size="sm" />
        <span>{{ error || successMessage }}</span>
      </div>

      <div class="app-workspace" :class="{ 'is-free-media': isFreeMediaStudio }">
        <div class="workspace-main">
          <section class="catalog-section" aria-labelledby="available-apps-title">
            <div class="section-heading">
              <div><h2 id="available-apps-title">可用应用</h2><span>{{ filteredApplications.length }} 个应用</span></div>
              <code v-if="selectedApplication">{{ selectedApplication.slug }}</code>
            </div>
            <div v-if="isLoading && !applications.length" class="catalog-loading"><span v-for="index in 3" :key="index" /></div>
            <div v-else-if="filteredApplications.length" class="catalog-grid">
              <AppCatalogCard
                v-for="application in filteredApplications"
                :key="application.slug"
                :application="application"
                :presentation="APP_PRESENTATIONS[application.slug as ApplicationSlug]"
                :selected="selectedSlug === application.slug"
                :disabled="isRunning"
                @select="handleSelectApplication(application.slug)"
              />
            </div>
            <div v-else class="catalog-empty"><Icon name="inbox" size="lg" /><strong>没有匹配的应用</strong><span>调整搜索词或应用类型。</span></div>
          </section>

          <AppRunStatus :run="activeRun" :steps="progressSteps" :running="isRunning" @cancel="cancelRun" />
          <FreeMediaStudioPanel v-if="isFreeMediaStudio" :runtime="freeMediaRuntime" />
          <AppResultPanel v-else :result="localResult" :running="isRunning" />
          <AppRunHistory
            :runs="history"
            :selected-run="selectedHistoryRun"
            :loading="isLoadingHistory"
            :video-preview-url="historyVideoPreviewUrl"
            :video-preview-loading="historyVideoPreviewLoading"
            :video-preview-error="historyVideoPreviewError"
            @refresh="loadHistory"
            @select="openHistoryRun"
            @resume="resumeRun"
          />
        </div>

        <aside v-if="!isFreeMediaStudio" class="runner-panel" aria-label="应用运行参数">
          <header class="runner-panel-header">
            <span class="runner-mark" :data-tone="selectedPresentation.category"><Icon :name="selectedPresentation.icon" size="md" /></span>
            <div><strong>{{ selectedApplication?.name || selectedPresentation.name }}</strong><span>{{ selectedApplication?.status || '可用' }} · v{{ selectedApplication?.version || selectedPresentation.version }}</span></div>
          </header>
          <AppKeySelectors
            v-model:planner-key-id="selectedPlannerKeyId"
            v-model:media-key-id="selectedMediaKeyId"
            v-model:planner-enabled="usePlanner"
            :planner-options="plannerKeyOptions"
            :media-options="mediaKeyOptions"
            :loading="isLoadingKeys"
            :disabled="isRunning"
          />
          <component
            :is="runnerComponent"
            :definition="selectedApplication"
            :media-platform="selectedMediaPlatform"
            :disabled="isRunning || !selectedApplication"
            @run="handleRun"
          />
        </aside>
      </div>
    </section>
  </AppLayout>
</template>

<style scoped>
.app-center {
  --apps-purple: #6d28d9;
  --customer-surface: #ffffff;
  --customer-surface-raised: #ffffff;
  --customer-surface-soft: #f3f6f9;
  --customer-ink: #17212b;
  --customer-muted: #647181;
  --customer-line: #cbd5df;
  --customer-line-strong: #94a3b8;
  --customer-cyan: #0f766e;
  --customer-cyan-soft: #e7f7f4;
  --customer-cyan-ink: #ffffff;
  --customer-cyan-shadow: rgb(15 118 110 / 18%);
  --customer-copper: #c2410c;
  --customer-copper-soft: #fff1e7;
  --customer-danger: #dc2626;
  display: grid;
  gap: 14px;
  color: var(--customer-ink);
}

.app-center-header { display: flex; min-height: 76px; align-items: center; justify-content: space-between; gap: 18px; padding: 0 2px; }
.header-kicker { color: var(--customer-copper); font-size: 10px; font-weight: 800; letter-spacing: 0; }
h1 { margin: 5px 0 0; font-size: 22px; font-weight: 760; line-height: 1.25; letter-spacing: 0; }
.catalog-tools { display: flex; align-items: center; gap: 8px; }
.search-control { display: flex; width: min(300px, 32vw); min-height: 38px; align-items: center; gap: 7px; padding: 0 10px; border: 1px solid var(--customer-line); border-radius: 6px; color: var(--customer-muted); background: var(--customer-surface); }
.search-control:focus-within { border-color: var(--customer-cyan); box-shadow: 0 0 0 3px var(--customer-cyan-shadow); }
.search-control input { width: 100%; min-width: 0; border: 0; outline: 0; color: var(--customer-ink); background: transparent; font-size: 11px; }
.catalog-tools select,
.catalog-tools > button { min-height: 38px; border: 1px solid var(--customer-line); border-radius: 6px; color: var(--customer-muted); background: var(--customer-surface); font-size: 11px; }
.catalog-tools select { min-width: 112px; padding: 7px 9px; }
.catalog-tools > button { display: grid; width: 38px; place-items: center; }
.catalog-tools > button:hover { border-color: var(--customer-cyan); color: var(--customer-cyan); }
.catalog-tools select:focus-visible,
.catalog-tools > button:focus-visible { border-color: var(--customer-cyan); outline: 3px solid var(--customer-cyan-shadow); outline-offset: 1px; }
.workspace-message { display: flex; align-items: center; gap: 7px; padding: 10px 12px; border: 1px solid rgb(59 140 117 / 30%); border-radius: 6px; color: #2f7764; background: rgb(59 140 117 / 8%); font-size: 11px; }
.workspace-message.error { border-color: rgb(199 90 82 / 30%); color: #b6413a; background: rgb(199 90 82 / 8%); }
.app-workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(390px, 0.47fr); align-items: start; gap: 14px; }
.app-workspace.is-free-media { grid-template-columns: minmax(0, 1fr); }
.workspace-main { display: grid; min-width: 0; gap: 14px; }
.catalog-section { min-width: 0; }
.section-heading { display: flex; min-height: 46px; align-items: center; justify-content: space-between; gap: 14px; padding: 2px 2px 10px; }
.section-heading > div { display: flex; align-items: baseline; gap: 8px; }
.section-heading h2 { margin: 0; font-size: 13px; font-weight: 740; }
.section-heading span,
.section-heading code { color: var(--customer-muted); font-size: 9px; }
.catalog-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
.catalog-loading { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
.catalog-loading span { min-height: 142px; border-radius: 7px; background: var(--customer-surface-soft); animation: loading-pulse 1.2s ease-in-out infinite; }
.catalog-empty { display: flex; min-height: 176px; flex-direction: column; align-items: center; justify-content: center; gap: 7px; color: var(--customer-muted); }
.catalog-empty strong { color: var(--customer-ink); font-size: 12px; }
.catalog-empty span { font-size: 10px; }
.runner-panel { position: sticky; top: 86px; display: grid; min-width: 0; gap: 14px; padding: 14px; border: 1px solid var(--customer-line); border-radius: 7px; background: var(--customer-surface); }
.runner-panel-header { display: grid; grid-template-columns: 42px minmax(0, 1fr); align-items: center; gap: 10px; padding-bottom: 12px; border-bottom: 1px solid var(--customer-line); }
.runner-mark { display: grid; width: 42px; height: 42px; place-items: center; border-radius: 6px; color: var(--customer-cyan); background: var(--customer-cyan-soft); }
.runner-mark[data-tone="commerce"] { color: var(--customer-copper); background: var(--customer-copper-soft); }
.runner-mark[data-tone="video"] { color: var(--apps-purple); background: rgb(111 90 167 / 12%); }
.runner-panel-header div { min-width: 0; }
.runner-panel-header strong,
.runner-panel-header span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.runner-panel-header strong { font-size: 13px; }
.runner-panel-header span { margin-top: 4px; color: var(--customer-muted); font-size: 9px; }
.spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@keyframes loading-pulse { 50% { opacity: .55; } }
@media (max-width: 1180px) { .app-workspace { grid-template-columns: 1fr; } .runner-panel { position: static; grid-row: 1; } }
@media (max-width: 760px) { .app-center-header { align-items: stretch; flex-direction: column; } .catalog-tools { display: grid; grid-template-columns: 1fr auto auto; } .search-control { width: 100%; } .catalog-grid,
.catalog-loading { grid-template-columns: 1fr; } }
@media (max-width: 470px) { .catalog-tools { grid-template-columns: 1fr 38px; } .search-control { grid-column: 1 / -1; } .catalog-tools select { min-width: 0; } }
@media (prefers-reduced-motion: reduce) { .spinning,
.catalog-loading span { animation: none; } }
</style>

<style>
.dark .app-center {
  --customer-surface: #111827;
  --customer-surface-raised: #172033;
  --customer-surface-soft: #1e293b;
  --customer-ink: #f1f5f9;
  --customer-muted: #a8b3c2;
  --customer-line: #475569;
  --customer-line-strong: #64748b;
  --customer-cyan: #2dd4bf;
  --customer-cyan-soft: rgb(45 212 191 / 14%);
  --customer-cyan-ink: #042f2e;
  --customer-cyan-shadow: rgb(45 212 191 / 20%);
  --customer-copper: #fb923c;
  --customer-copper-soft: rgb(251 146 60 / 14%);
  --customer-danger: #f87171;
}
</style>
