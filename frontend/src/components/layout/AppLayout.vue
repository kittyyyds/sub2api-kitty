<template>
  <div
    class="kitty-console-shell bg-gray-50 dark:bg-dark-950"
    :class="[isAdmin ? 'kitty-admin-shell' : 'kitty-customer-shell', contained ? 'app-layout-contained h-dvh overflow-hidden' : 'min-h-screen']"
    data-console-shell="true"
    :data-console-performance="consolePerformanceMode"
  >
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="kitty-console-main relative min-w-0 transition-[margin] duration-200"
      :class="[contained ? 'flex h-full min-h-0 flex-col' : 'min-h-screen', isAdmin
        ? (sidebarCollapsed ? 'lg:ml-[68px]' : 'lg:ml-[230px]')
        : (sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64')]"
    >
      <!-- Header -->
      <AppHeader :class="{ 'shrink-0': contained }" />

      <!-- Main Content -->
      <main class="kitty-console-content app-content w-full min-w-0" :class="[isAdmin ? 'p-3 sm:p-4 lg:p-5 xl:p-6' : 'p-4 md:p-6 lg:p-8', { 'min-h-0 flex-1 overflow-hidden': contained }]">
        <CustomerPageTabs v-if="!isAdmin" />
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import '@/styles/console-admin-theme.css'
import '@/styles/console-theme.css'
import '@/styles/cs1-provider-theme.css'
import '@/styles/relay.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import { detectConsolePerformanceMode } from '@/utils/consolePerformance'
import { useConsoleTheme } from '@/composables/useConsoleTheme'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import CustomerPageTabs from './CustomerPageTabs.vue'

// Opt in for pages whose panels own their scrolling, while keeping the
// ordinary document layout for all other routes.
withDefaults(defineProps<{ contained?: boolean }>(), { contained: false })

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.isAdmin)
const consolePerformanceMode = detectConsolePerformanceMode()
useConsoleTheme()

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
/* Flex/grid children must shrink with the available viewport instead of
   widening the whole page when a table or long label has intrinsic width. */
.app-content > :deep(*) {
  min-width: 0;
}

/* Mobile browser chrome can make 100vh taller than the visible viewport.
   Override the default body minimum only while a contained page is mounted. */
:global(body:has(.app-layout-contained)) {
  min-height: 100dvh;
}
</style>
