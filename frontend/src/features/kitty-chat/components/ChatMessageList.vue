<script setup lang="ts">
import { computed } from 'vue'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

import Icon from '@/components/icons/Icon.vue'

import type { ChatMessage } from '../types'

const props = defineProps<{
  messages: ChatMessage[]
  logo: string
  streaming: boolean
}>()

defineEmits<{
  regenerate: []
}>()

const renderedMessages = computed(() => props.messages.map((message) => ({
  ...message,
  html: message.role === 'assistant' && message.content
    ? DOMPurify.sanitize(marked.parse(message.content) as string)
    : '',
})))

function formattedTime(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' }).format(value)
}
</script>

<template>
  <section class="message-list" aria-live="polite">
    <article v-for="message in renderedMessages" :key="message.id" class="message" :class="message.role">
      <div class="message-avatar">
        <img v-if="message.role === 'assistant'" :src="logo || '/logo.svg'" alt="AI">
        <Icon v-else name="user" size="sm" />
      </div>
      <div class="message-body">
        <header>
          <strong>{{ message.role === 'assistant' ? 'Kitty AI' : '你' }}</strong>
          <time>{{ formattedTime(message.createdAt) }}</time>
        </header>
        <details v-if="message.reasoning" class="reasoning">
          <summary>查看思考过程</summary>
          <pre>{{ message.reasoning }}</pre>
        </details>
        <div v-if="message.role === 'assistant' && message.content" class="markdown" v-html="message.html"></div>
        <p v-else-if="message.content" class="plain-text">{{ message.content }}</p>
        <div v-else-if="message.role === 'assistant' && streaming" class="typing" aria-label="正在生成">
          <span></span><span></span><span></span>
        </div>
        <ul v-if="message.attachments?.length" class="attachment-list">
          <li v-for="attachment in message.attachments" :key="`${message.id}-${attachment.name}`">
            <Icon name="document" size="xs" />{{ attachment.name }}
          </li>
        </ul>
        <p v-if="message.error" class="message-error" role="alert">{{ message.error }}</p>
      </div>
    </article>

    <button
      v-if="messages.some((message) => message.role === 'assistant') && !streaming"
      class="regenerate"
      type="button"
      @click="$emit('regenerate')"
    >
      <Icon name="refresh" size="sm" />
      重新生成上一条回复
    </button>
  </section>
</template>

<style scoped>
.message-list {
  width: min(920px, calc(100% - 32px));
  margin: 0 auto;
  padding: 24px 0 190px;
}

.message {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 11px;
  padding: 16px 0;
}

.message + .message {
  border-top: 1px solid color-mix(in srgb, var(--workbench-line) 65%, transparent);
}

.message-avatar {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--workbench-line);
  border-radius: 7px;
  color: var(--workbench-muted);
  background: var(--workbench-surface);
}

.message-avatar img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.message-body {
  min-width: 0;
}

.message-body header {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 8px;
}

.message-body header strong {
  color: var(--workbench-ink);
  font-size: 11px;
}

.message-body time {
  color: var(--workbench-muted);
  font-size: 9px;
}

.plain-text,
.markdown {
  color: var(--workbench-ink);
  font-size: 12px;
  line-height: 1.75;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.markdown :deep(p),
.markdown :deep(ul),
.markdown :deep(ol),
.markdown :deep(pre),
.markdown :deep(blockquote) {
  margin: 0 0 10px;
}

.markdown :deep(pre) {
  overflow-x: auto;
  border: 1px solid var(--workbench-line);
  border-radius: 6px;
  padding: 12px;
  background: var(--workbench-surface-soft);
  font-size: 11px;
}

.markdown :deep(code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.markdown :deep(:not(pre) > code) {
  border-radius: 4px;
  padding: 2px 5px;
  background: var(--workbench-surface-soft);
}

.markdown :deep(a) {
  color: var(--workbench-accent);
  text-decoration: underline;
}

.reasoning {
  margin-bottom: 10px;
  border-left: 2px solid var(--workbench-accent);
  padding-left: 10px;
  color: var(--workbench-muted);
  font-size: 10px;
}

.reasoning summary {
  cursor: pointer;
  color: var(--workbench-accent);
}

.reasoning pre {
  margin: 8px 0 0;
  font-family: inherit;
  line-height: 1.6;
  white-space: pre-wrap;
}

.attachment-list {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 10px;
}

.attachment-list li {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--workbench-line);
  border-radius: 5px;
  padding: 4px 7px;
  color: var(--workbench-muted);
  font-size: 9px;
}

.message-error {
  margin-top: 8px;
  color: #dc2626;
  font-size: 10px;
}

.typing {
  display: flex;
  gap: 4px;
  padding-top: 7px;
}

.typing span {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--workbench-accent);
  animation: typing 1s ease-in-out infinite;
}

.typing span:nth-child(2) { animation-delay: .14s; }
.typing span:nth-child(3) { animation-delay: .28s; }

.regenerate {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 14px auto 0;
  border: 1px solid var(--workbench-line);
  border-radius: 6px;
  padding: 7px 10px;
  color: var(--workbench-muted);
  font-size: 10px;
}

.regenerate:hover {
  border-color: var(--workbench-accent);
  color: var(--workbench-accent);
}

@keyframes typing {
  0%, 70%, 100% { opacity: .3; transform: translateY(0); }
  35% { opacity: 1; transform: translateY(-3px); }
}

@media (prefers-reduced-motion: reduce) {
  .typing span { animation: none; opacity: 1; }
}
</style>
