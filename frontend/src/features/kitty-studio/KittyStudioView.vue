<script setup lang="ts">
import { computed, onMounted } from 'vue'

import AppLayout from '@/components/layout/AppLayout.vue'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import { useAppStore } from '@/stores/app'

import StudioCanvas from './components/StudioCanvas.vue'
import StudioControls from './components/StudioControls.vue'
import { useKittyStudio } from './useKittyStudio'

const appStore = useAppStore()
const studio = useKittyStudio()
const { isDarkMode } = useDocumentDarkMode()
const logo = computed(() => appStore.brandLogoUrl || '/logo.svg')

onMounted(() => {
  if (!appStore.publicSettingsLoaded) void appStore.fetchPublicSettings()
  void Promise.all([studio.loadKeys(), studio.loadGallery()])
})
</script>

<template>
  <AppLayout>
    <section class="kitty-studio-shell" :class="{ 'is-dark': isDarkMode }" aria-label="Kitty 生图工作室">
      <StudioControls
        :form="studio.form"
        :keys="studio.availableKeys.value"
        :models="studio.modelOptions.value"
        :reference-previews="studio.referencePreviews.value"
        :selected-platform="studio.selectedPlatform.value"
        :loading-keys="studio.loadingKeys.value"
        :loading-models="studio.loadingModels.value"
        :generating="studio.generating.value"
        :can-generate="studio.canGenerate.value"
        :needs-reference="studio.needsReference.value"
        :video-mode="studio.videoMode.value"
        @mode="studio.setMode"
        @key="studio.setKey"
        @update="studio.updateForm"
        @files="studio.addReferences"
        @remove-reference="studio.removeReference"
        @generate="studio.generate"
        @stop="studio.stop"
        @refresh="studio.loadKeys"
      />

      <StudioCanvas
        :logo="logo"
        :gallery="studio.gallery.value"
        :progress="studio.progress.value"
        :loading="studio.loadingGallery.value"
        :generating="studio.generating.value"
        :error="studio.error.value"
        :notice="studio.notice.value"
        @clear="studio.clearGallery"
        @download="studio.downloadResult"
        @copy="studio.copyPrompt"
        @remove="studio.removeResult"
      />
    </section>
  </AppLayout>
</template>

<style scoped>
.kitty-studio-shell {
  --studio-bg: #f4f7f9;
  --studio-panel: #fbfcfd;
  --studio-panel-soft: #eef2f5;
  --studio-surface: #ffffff;
  --studio-ink: #18222d;
  --studio-muted: #6d7b89;
  --studio-line: #dce3e8;
  --studio-line-strong: #b9c5ce;
  --studio-accent: #0f766e;
  --studio-accent-strong: #115e59;
  --studio-accent-soft: #e6f6f3;
  --studio-accent-shadow: rgb(15 118 110 / 18%);
  --studio-ring: rgb(15 118 110 / 14%);
  display: grid;
  grid-template-columns: 350px minmax(0, 1fr);
  min-height: min(820px, calc(100dvh - 118px));
  overflow: hidden;
  border: 1px solid var(--studio-line);
  border-radius: 8px;
  color: var(--studio-ink);
  background: var(--studio-bg);
  box-shadow: 0 18px 44px rgb(15 23 42 / 8%);
}

.kitty-studio-shell.is-dark {
  --studio-bg: #09111c;
  --studio-panel: #101925;
  --studio-panel-soft: #152131;
  --studio-surface: #111c2a;
  --studio-ink: #eef3f7;
  --studio-muted: #99a7b7;
  --studio-line: #283649;
  --studio-line-strong: #3c4d62;
  --studio-accent: #2dd4bf;
  --studio-accent-strong: #5eead4;
  --studio-accent-soft: rgb(45 212 191 / 10%);
  --studio-accent-shadow: rgb(45 212 191 / 14%);
  --studio-ring: rgb(45 212 191 / 15%);
  box-shadow: 0 20px 60px rgb(0 0 0 / 30%);
}

@media (max-width: 1040px) {
  .kitty-studio-shell {
    grid-template-columns: 1fr;
    overflow: visible;
  }
}

@media (max-width: 640px) {
  .kitty-studio-shell {
    border-radius: 6px;
  }
}
</style>
