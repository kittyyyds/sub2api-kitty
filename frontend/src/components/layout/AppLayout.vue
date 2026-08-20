<template>
  <div
    class="kitty-console-shell min-h-screen bg-gray-50 dark:bg-dark-950"
    data-console-shell="true"
    :data-console-performance="consolePerformanceMode"
  >
    <!-- Background Decoration -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="kitty-console-main relative min-h-screen transition-[margin] duration-200"
      :class="[sidebarCollapsed ? 'lg:ml-[68px]' : 'lg:ml-[230px]']"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main class="kitty-console-content p-3 sm:p-4 lg:p-5 xl:p-6">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import '@/styles/console-theme.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import { detectConsolePerformanceMode } from '@/utils/consolePerformance'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')
const consolePerformanceMode = detectConsolePerformanceMode()

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
