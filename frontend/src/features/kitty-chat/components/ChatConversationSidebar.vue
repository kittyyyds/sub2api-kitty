<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

import type { ChatConversation } from '../types'

defineProps<{
  conversations: ChatConversation[]
  activeId: string
  disabled: boolean
}>()

defineEmits<{
  create: []
  select: [id: string]
  delete: [id: string]
  clear: []
}>()
</script>

<template>
  <aside class="conversation-sidebar" aria-label="本地对话列表">
    <button class="new-chat" type="button" :disabled="disabled" @click="$emit('create')">
      <Icon name="plus" size="sm" />
      新建对话
    </button>

    <div class="conversation-list">
      <article
        v-for="conversation in conversations"
        :key="conversation.id"
        class="conversation-item"
        :class="{ active: activeId === conversation.id }"
      >
        <button type="button" :disabled="disabled" @click="$emit('select', conversation.id)">
          <strong>{{ conversation.title }}</strong>
          <span>{{ conversation.model || '尚未选择模型' }}</span>
        </button>
        <button
          class="delete-conversation"
          type="button"
          title="删除对话"
          :aria-label="`删除对话 ${conversation.title}`"
          :disabled="disabled"
          @click="$emit('delete', conversation.id)"
        >
          <Icon name="trash" size="xs" />
        </button>
      </article>
    </div>

    <footer class="conversation-footer">
      <span>{{ conversations.length }} 个对话</span>
      <button type="button" :disabled="disabled" @click="$emit('clear')">清空全部</button>
    </footer>
  </aside>
</template>

<style scoped>
.conversation-sidebar {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  min-width: 0;
  min-height: 680px;
  border-right: 1px solid var(--workbench-line);
  background: color-mix(in srgb, var(--workbench-surface) 88%, transparent);
}

.new-chat {
  display: flex;
  min-height: 42px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin: 12px;
  border: 1px dashed var(--workbench-line-strong);
  border-radius: 7px;
  color: var(--workbench-ink);
  font-size: 12px;
  font-weight: 700;
}

.new-chat:hover:not(:disabled) {
  border-color: var(--workbench-accent);
  color: var(--workbench-accent);
  background: var(--workbench-accent-soft);
}

.conversation-list {
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px 12px;
}

.conversation-item {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 30px;
  align-items: center;
  border-radius: 6px;
}

.conversation-item.active {
  background: var(--workbench-accent-soft);
}

.conversation-item > button:first-child {
  min-width: 0;
  padding: 10px 8px;
  text-align: left;
}

.conversation-item strong,
.conversation-item span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conversation-item strong {
  color: var(--workbench-ink);
  font-size: 11px;
  font-weight: 650;
}

.conversation-item span {
  margin-top: 4px;
  color: var(--workbench-muted);
  font-size: 9px;
}

.delete-conversation {
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: 5px;
  color: var(--workbench-muted);
  opacity: 0;
}

.conversation-item:hover .delete-conversation,
.delete-conversation:focus-visible {
  opacity: 1;
}

.delete-conversation:hover:not(:disabled) {
  color: #dc2626;
  background: rgb(220 38 38 / 9%);
}

.conversation-footer {
  display: flex;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  border-top: 1px solid var(--workbench-line);
  color: var(--workbench-muted);
  font-size: 10px;
}

.conversation-footer button:hover:not(:disabled) {
  color: var(--workbench-accent);
}

@media (max-width: 960px) {
  .conversation-sidebar {
    grid-template-rows: auto auto;
    min-height: 0;
    border-right: 0;
    border-bottom: 1px solid var(--workbench-line);
  }

  .new-chat {
    margin: 10px 10px 6px;
  }

  .conversation-list {
    display: flex;
    gap: 6px;
    overflow-x: auto;
    padding: 0 10px 10px;
  }

  .conversation-item {
    flex: 0 0 190px;
  }

  .conversation-footer {
    display: none;
  }
}
</style>
