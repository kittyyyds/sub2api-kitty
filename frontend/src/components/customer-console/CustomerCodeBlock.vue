<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    code: string
    label: string
    language?: string
    compact?: boolean
  }>(),
  {
    language: 'text',
    compact: false
  }
)

type CopyState = 'copied' | 'error' | 'idle'

const copyState = shallowRef<CopyState>('idle')
let resetTimer: ReturnType<typeof setTimeout> | undefined

const copyLabel = computed(() => {
  if (copyState.value === 'copied') return t('native.codeBlock.copied')
  if (copyState.value === 'error') return t('native.codeBlock.failed')
  return t('native.codeBlock.copy')
})

function legacyCopy(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.inset = '-9999px auto auto -9999px'
  document.body.appendChild(textarea)
  textarea.select()

  try {
    return document.execCommand('copy')
  } finally {
    textarea.remove()
  }
}

async function copyCode(): Promise<void> {
  if (resetTimer) clearTimeout(resetTimer)

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(props.code)
    } else if (!legacyCopy(props.code)) {
      throw new Error('Clipboard unavailable')
    }
    copyState.value = 'copied'
  } catch {
    copyState.value = 'error'
  }

  resetTimer = setTimeout(() => {
    copyState.value = 'idle'
  }, 1800)
}

onBeforeUnmount(() => {
  if (resetTimer) clearTimeout(resetTimer)
})
</script>

<template>
  <section class="code-block" :class="{ 'code-block--compact': compact }">
    <div class="code-block__toolbar">
      <div class="code-block__identity">
        <span class="code-block__lights" aria-hidden="true">
          <i></i>
          <i></i>
          <i></i>
        </span>
        <span class="code-block__label">{{ label }}</span>
        <span class="code-block__language">{{ language }}</span>
      </div>

      <button
        class="code-block__copy"
        :class="{ 'code-block__copy--done': copyState === 'copied' }"
        type="button"
        :aria-label="t('native.codeBlock.copyAria', { label })"
        :title="t('native.codeBlock.copyAria', { label })"
        @click="copyCode"
      >
        <Icon :name="copyState === 'copied' ? 'check' : 'copy'" size="sm" :stroke-width="1.8" />
        <span aria-live="polite">{{ copyLabel }}</span>
      </button>
    </div>

    <pre class="code-block__pre"><code>{{ code }}</code></pre>
  </section>
</template>

<style scoped>
.code-block {
  min-width: 0;
  overflow: hidden;
  color: #dcebed;
  background: #071216;
  border: 1px solid var(--customer-line, rgba(224, 235, 232, 0.11));
  border-radius: 7px;
  box-shadow: 0 12px 30px rgba(1, 7, 9, 0.2);
}

.code-block__toolbar {
  display: flex;
  min-height: 42px;
  gap: 14px;
  align-items: center;
  justify-content: space-between;
  padding: 7px 9px 7px 12px;
  background: #0d1d22;
  border-bottom: 1px solid rgba(224, 235, 232, 0.1);
}

.code-block__identity {
  display: flex;
  min-width: 0;
  gap: 9px;
  align-items: center;
}

.code-block__lights {
  display: flex;
  flex: 0 0 auto;
  gap: 4px;
}

.code-block__lights i {
  display: block;
  width: 5px;
  height: 5px;
  background: #557078;
  border-radius: 50%;
}

.code-block__lights i:nth-child(2) {
  background: #5fa9b4;
}

.code-block__lights i:nth-child(3) {
  background: #b77e48;
}

.code-block__label {
  overflow: hidden;
  color: #eef7f7;
  font-size: 12px;
  font-weight: 650;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.code-block__language {
  flex: 0 0 auto;
  color: #779199;
  font-family: var(--customer-mono, monospace);
  font-size: 10px;
  line-height: 1;
  text-transform: uppercase;
}

.code-block__copy {
  display: inline-flex;
  min-width: 76px;
  min-height: 28px;
  gap: 6px;
  align-items: center;
  justify-content: center;
  padding: 5px 8px;
  color: #a9c3c8;
  font: inherit;
  font-size: 11px;
  font-weight: 650;
  cursor: pointer;
  background: rgba(142, 234, 255, 0.06);
  border: 1px solid rgba(142, 234, 255, 0.16);
  border-radius: 5px;
  transition:
    color 160ms ease,
    background-color 160ms ease,
    border-color 160ms ease;
}

.code-block__copy:hover {
  color: #e8fbff;
  background: rgba(142, 234, 255, 0.12);
  border-color: rgba(142, 234, 255, 0.34);
}

.code-block__copy:focus-visible {
  outline: 2px solid #8eeaff;
  outline-offset: 2px;
}

.code-block__copy--done {
  color: #ffca9c;
  background: rgba(255, 173, 104, 0.1);
  border-color: rgba(255, 173, 104, 0.3);
}

.code-block__pre {
  max-width: 100%;
  min-height: 132px;
  max-height: 420px;
  padding: 18px;
  margin: 0;
  overflow: auto;
  font-family: var(--customer-mono, 'Cascadia Code', Consolas, monospace);
  font-size: 12px;
  line-height: 1.72;
  scrollbar-color: #3e626b #071216;
  tab-size: 2;
}

.code-block__pre code {
  font: inherit;
  white-space: pre;
}

.code-block--compact .code-block__pre {
  min-height: 0;
  padding: 13px 16px;
}

@media (max-width: 560px) {
  .code-block__toolbar {
    align-items: flex-start;
  }

  .code-block__identity {
    flex-wrap: wrap;
    gap: 5px 8px;
  }

  .code-block__lights {
    display: none;
  }

  .code-block__label {
    flex-basis: 100%;
    white-space: normal;
  }

  .code-block__copy {
    min-width: 36px;
  }

  .code-block__copy span {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  .code-block__pre {
    padding: 14px;
    font-size: 11px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .code-block__copy {
    transition: none;
  }
}
</style>
