<template>
  <!-- 后台内嵌形态:?embedded=1 且已登录,套完整后台布局 -->
  <div v-if="isCompact" class="kitty-media-compact">
    <MediaPricingCatalog :response="data" :loading="loading" :error="loadFailed" :catalog-policy="suanliMediaCatalog" :received-at="receivedAt" :personal="authStore.isAuthenticated" :initial-kind="initialKind" :initial-query="initialQuery" :initial-group-id="initialGroupId" :initial-detail="initialDetail" @refresh="loadData" />
  </div>
  <AppLayout v-else-if="isEmbedded">
    <CatalogViewSwitch v-model="view" />
    <MediaPricingCatalog v-if="view === 'media'" :response="data" :loading="loading" :error="loadFailed"
      :catalog-policy="suanliMediaCatalog" :received-at="receivedAt" :personal="authStore.isAuthenticated" :initial-kind="initialKind" :initial-query="initialQuery" :initial-group-id="initialGroupId" :initial-detail="initialDetail" @refresh="loadData" />
    <ModelPlazaContent
      v-else
      :response="generalData"
      :loading="loading"
      :error="loadFailed"
      embedded
      @retry="loadData"
    />
  </AppLayout>

  <!-- 独立形态:自带导航条(logo/站名 + 登录/回后台) -->
  <div v-else class="kitty-media-page relative z-10 min-h-screen">
    <PlazaNavBar />
    <main class="mx-auto w-full max-w-[1600px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
      <CatalogViewSwitch v-model="view" />
      <MediaPricingCatalog v-if="view === 'media'" :response="data" :loading="loading" :error="loadFailed"
        :catalog-policy="suanliMediaCatalog" :received-at="receivedAt" :personal="authStore.isAuthenticated" :initial-kind="initialKind" :initial-query="initialQuery" :initial-group-id="initialGroupId" :initial-detail="initialDetail" @refresh="loadData" />
      <ModelPlazaContent
        v-else
        :response="generalData"
        :loading="loading"
        :error="loadFailed"
        @retry="loadData"
      />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import CatalogViewSwitch from '@/components/modelPlaza/CatalogViewSwitch.vue'
import { generalPricingResponse } from '@/components/modelPlaza/generalPricing'
import ModelPlazaContent from '@/components/modelPlaza/ModelPlazaContent.vue'
import MediaPricingCatalog from '@/components/modelPlaza/MediaPricingCatalog.vue'
import { suanliMediaCatalog } from '@/components/modelPlaza/mediaCatalogPolicy'
import { getModelPlaza, type ModelPlazaResponse } from '@/api/modelPlaza'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import '@/styles/console-theme.css'
import { useConsoleTheme } from '@/composables/useConsoleTheme'
useConsoleTheme()

const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const view = ref<'all' | 'media'>(route.query.media === '1' ? 'media' : 'all')
const initialKind = computed(() => route.query.type === 'video' ? 'video' : 'image')
const initialQuery = computed(() => typeof route.query.q === 'string' ? route.query.q : '')
const initialGroupId = computed(() => { const value = Number(route.query.group); return Number.isSafeInteger(value) && value > 0 ? value : undefined })
const initialDetail = computed(() => typeof route.query.detail === 'string' ? route.query.detail : '')
const isCompact = computed(() => route.query.compact === '1' && route.query.media === '1')

// embedded=1 但未登录(如转发的链接)自动降级为独立形态。
const isEmbedded = computed(() => route.query.embedded === '1' && authStore.isAuthenticated)

const data = ref<ModelPlazaResponse | null>(null)
const generalData = computed(() => generalPricingResponse(data.value))
const loading = ref(true)
const loadFailed = ref(false)
const receivedAt = ref('')
let controller: AbortController | undefined
let timer: ReturnType<typeof setInterval> | undefined
let requestId = 0

async function loadData(): Promise<void> {
  const id = ++requestId
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  loadFailed.value = false
  try {
    const next = await getModelPlaza({ signal: controller.signal })
    if (id === requestId) { data.value = next; receivedAt.value = new Date().toISOString() }
  } catch {
    if (id === requestId) loadFailed.value = true
  } finally {
    if (id === requestId) loading.value = false
  }
}
function refreshVisible() { if (!document.hidden && !loading.value) void loadData() }
watch(() => authStore.user?.id, () => { data.value = null; receivedAt.value = ''; void loadData() })

onMounted(() => {
  // 独立形态导航条需要站点名/Logo;有 __APP_CONFIG__ 注入时同步命中缓存。
  void appStore.fetchPublicSettings()
  void loadData()
  timer = setInterval(refreshVisible, 60000)
  document.addEventListener('visibilitychange', refreshVisible)
})
onBeforeUnmount(() => { requestId++; controller?.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', refreshVisible) })
</script>

<style scoped>
.kitty-media-page, .kitty-media-compact { background: var(--kc-bg); color: var(--kc-ink); font-family: var(--kc-font); }
.kitty-media-compact { min-height: 100vh; padding: 20px; }
</style>
