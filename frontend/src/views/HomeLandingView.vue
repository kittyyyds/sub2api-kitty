<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

import HeyRouteLandingPage from '@/components/home/heyroute/HeyRouteLandingPage.vue'
import { heyRouteCopyByLanguage, withSiteName } from '@/components/home/heyroute/heyRouteCopy'
import { useHomePricingPreview } from '@/composables/useHomePricingPreview'
import { useTheme } from '@/composables/useTheme'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()
const authStore = useAuthStore()
const { locale } = useI18n()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Kitty奇妙屋')
// The copy is written with the default brand name; show the operator's configured name instead.
const copy = computed(() => withSiteName(heyRouteCopyByLanguage[locale.value.startsWith('zh') ? 'zh' : 'en'], siteName.value))
const siteLogo = computed(() => appStore.brandLogoUrl)
const docUrl = computed(() => sanitizeUrl(
  appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '',
))
const apiBaseUrl = computed(() => {
  const value = sanitizeUrl(
    appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || window.location.origin,
  ) || window.location.origin
  return value.replace(/\/$/, '')
})
const modelPlazaEnabled = computed(() => appStore.cachedPublicSettings?.model_plaza_enabled === true)
const isAuthenticated = computed(() => authStore.isAuthenticated)
// A catalog that requires login answers 401 to visitors; do not request it for them.
const pricingPreviewEnabled = computed(() => modelPlazaEnabled.value
  && (appStore.cachedPublicSettings?.model_plaza_require_auth !== true || isAuthenticated.value))
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const { isDark, toggleTheme } = useTheme()

const {
  summaries,
  loading,
  failed,
  reload,
} = useHomePricingPreview({ enabled: pricingPreviewEnabled })

// The router guard already restored the session on the first navigation; no second /auth/me here.
onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }
})
</script>

<template>
  <HeyRouteLandingPage
    :copy="copy"
    :site-name="siteName"
    :site-logo="siteLogo"
    :doc-url="docUrl"
    :api-base-url="apiBaseUrl"
    :is-dark="isDark"
    :is-authenticated="isAuthenticated"
    :dashboard-path="dashboardPath"
    :model-plaza-enabled="modelPlazaEnabled"
    :pricing-summaries="summaries"
    :pricing-loading="loading"
    :pricing-failed="failed"
    @toggle-theme="toggleTheme"
    @retry-pricing="reload"
  />
</template>
