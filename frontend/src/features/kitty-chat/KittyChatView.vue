<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import { useAppStore } from '@/stores/app'

import ChatComposer from './components/ChatComposer.vue'
import ChatConversationSidebar from './components/ChatConversationSidebar.vue'
import ChatMessageList from './components/ChatMessageList.vue'
import ChatSettingsDialog from './components/ChatSettingsDialog.vue'
import ChatToolbar from './components/ChatToolbar.vue'
import ChatWelcome from './components/ChatWelcome.vue'
import { useKittyChat } from './useKittyChat'

const appStore = useAppStore()
const settingsOpen = shallowRef(false)
const { isDarkMode } = useDocumentDarkMode()

const chat = useKittyChat()
const logo = computed(() => appStore.brandLogoUrl || '/logo.svg')
const messages = computed(() => chat.activeConversation.value?.messages || [])
const usageText = computed(() => chat.usage.value
  ? `${chat.usage.value.totalTokens.toLocaleString()} tokens`
  : '')

function useSuggestion(value: string): void {
  chat.draft.value = value
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) void appStore.fetchPublicSettings()
  void chat.loadKeys()
})
</script>

<template>
  <AppLayout>
    <section class="kitty-chat-shell" :class="{ 'is-dark': isDarkMode }" aria-label="Kitty 在线聊天">
      <ChatConversationSidebar
        :conversations="chat.conversations.value"
        :active-id="chat.activeConversationId.value"
        :disabled="chat.streaming.value"
        @create="chat.createConversation"
        @select="chat.selectConversation"
        @delete="chat.deleteConversation"
        @clear="chat.clearConversations"
      />

      <div class="chat-workspace">
        <ChatToolbar
          :keys="chat.availableKeys.value"
          :models="chat.models.value"
          :selected-key-id="chat.selectedKeyId.value"
          :selected-model="chat.selectedModel.value"
          :loading-keys="chat.loadingKeys.value"
          :loading-models="chat.loadingModels.value"
          :disabled="chat.streaming.value"
          @update:key="chat.setSelectedKey"
          @update:model="chat.setSelectedModel"
          @refresh="chat.loadKeys"
          @settings="settingsOpen = true"
          @export:markdown="chat.exportConversation('markdown')"
          @export:json="chat.exportConversation('json')"
        />

        <div class="chat-scroll">
          <div v-if="chat.error.value" class="chat-alert" role="alert">
            <Icon name="exclamationCircle" size="sm" />
            <span>{{ chat.error.value }}</span>
          </div>
          <ChatWelcome v-if="!messages.length" :logo="logo" @prompt="useSuggestion" />
          <ChatMessageList
            v-else
            :messages="messages"
            :logo="logo"
            :streaming="chat.streaming.value"
            @regenerate="chat.regenerateLast"
          />
        </div>

        <ChatComposer
          v-model="chat.draft.value"
          :attachments="chat.attachments.value"
          :can-send="chat.canSend.value"
          :streaming="chat.streaming.value"
          :usage-text="usageText"
          :endpoint-host="chat.endpointHost.value"
          @files="chat.addAttachments"
          @remove="chat.removeAttachment"
          @send="chat.sendMessage"
          @stop="chat.stopGeneration"
        />
      </div>
    </section>

    <ChatSettingsDialog
      :open="settingsOpen"
      :settings="chat.settings.value"
      @close="settingsOpen = false"
      @save="chat.updateSettings"
    />
  </AppLayout>
</template>

<style scoped>
.kitty-chat-shell {
  --workbench-bg: #f7fafc;
  --workbench-surface: #ffffff;
  --workbench-surface-soft: #eef3f7;
  --workbench-ink: #17212b;
  --workbench-muted: #687789;
  --workbench-line: #d8e0e8;
  --workbench-line-strong: #aebdca;
  --workbench-accent: #0f766e;
  --workbench-accent-strong: #115e59;
  --workbench-accent-soft: #e6f6f3;
  --workbench-accent-ring: rgb(15 118 110 / 14%);
  display: grid;
  grid-template-columns: 236px minmax(0, 1fr);
  min-height: min(760px, calc(100dvh - 118px));
  overflow: hidden;
  border: 1px solid var(--workbench-line);
  border-radius: 8px;
  color: var(--workbench-ink);
  background: var(--workbench-bg);
  box-shadow: 0 18px 42px rgb(15 23 42 / 8%);
}

.chat-workspace {
  position: relative;
  display: grid;
  min-width: 0;
  grid-template-rows: auto minmax(0, 1fr);
}

.chat-scroll {
  min-height: 0;
  overflow-y: auto;
}

.chat-alert {
  display: flex;
  width: min(820px, calc(100% - 32px));
  align-items: flex-start;
  gap: 7px;
  margin: 12px auto 0;
  border: 1px solid rgb(220 38 38 / 25%);
  border-radius: 6px;
  padding: 9px 11px;
  color: #b91c1c;
  background: rgb(220 38 38 / 7%);
  font-size: 10px;
}

.kitty-chat-shell.is-dark {
  --workbench-bg: #090f1f;
  --workbench-surface: #111827;
  --workbench-surface-soft: #172033;
  --workbench-ink: #edf2f7;
  --workbench-muted: #9aa8ba;
  --workbench-line: #273449;
  --workbench-line-strong: #3d4c62;
  --workbench-accent: #2dd4bf;
  --workbench-accent-strong: #5eead4;
  --workbench-accent-soft: rgb(45 212 191 / 11%);
  --workbench-accent-ring: rgb(45 212 191 / 15%);
  box-shadow: 0 20px 55px rgb(0 0 0 / 28%);
}

@media (max-width: 960px) {
  .kitty-chat-shell {
    grid-template-columns: 1fr;
    grid-template-rows: auto minmax(680px, 1fr);
  }
}

@media (max-width: 640px) {
  .kitty-chat-shell {
    min-height: calc(100dvh - 94px);
    border-radius: 6px;
  }
}
</style>
