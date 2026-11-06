<template>
  <div
    class="kitty-console-shell min-h-screen bg-gray-50 dark:bg-dark-950"
    :class="isAdmin ? 'kitty-admin-shell' : 'kitty-customer-shell'"
    data-console-shell="true"
    :data-console-performance="consolePerformanceMode"
  >
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="kitty-console-main relative min-h-screen transition-[margin] duration-200"
      :class="isAdmin
        ? (sidebarCollapsed ? 'lg:ml-[68px]' : 'lg:ml-[230px]')
        : (sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64')"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main class="kitty-console-content" :class="isAdmin ? 'p-3 sm:p-4 lg:p-5 xl:p-6' : 'p-4 md:p-6 lg:p-8'">
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
