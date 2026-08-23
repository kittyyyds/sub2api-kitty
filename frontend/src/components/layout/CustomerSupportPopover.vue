<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import { customerSupportContacts } from '@/config/customerContact'

const { t } = useI18n()
const rootRef = shallowRef<HTMLElement | null>(null)
const popoverRef = shallowRef<HTMLElement | null>(null)
const isOpen = shallowRef(false)
const isPinned = shallowRef(false)
const hasOpened = shallowRef(false)
const popoverStyle = shallowRef<Record<string, string>>({})
let closeTimer: ReturnType<typeof setTimeout> | undefined

function clearCloseTimer(): void {
  if (!closeTimer) return
  clearTimeout(closeTimer)
  closeTimer = undefined
}

async function positionPopover(): Promise<void> {
  await nextTick()
  const root = rootRef.value
  if (!root || !isOpen.value) return

  const rootRect = root.getBoundingClientRect()
  const width = Math.min(460, Math.max(280, window.innerWidth - 24))
  const preferredLeft = rootRect.left + (rootRect.width - width) / 2
  const left = Math.min(Math.max(preferredLeft, 12), window.innerWidth - width - 12)
  const popoverHeight = popoverRef.value?.offsetHeight ?? 360
  const spaceBelow = window.innerHeight - rootRect.bottom - 12
  const spaceAbove = rootRect.top - 12
  let top = rootRect.bottom + 10

  if (popoverHeight > spaceBelow && spaceAbove > spaceBelow) {
    top = Math.max(12, rootRect.top - popoverHeight - 10)
  }

  popoverStyle.value = {
    width: `${width}px`,
    left: `${left}px`,
    top: `${top}px`,
  }
}

function open(): void {
  clearCloseTimer()
  hasOpened.value = true
  isOpen.value = true
  void positionPopover()
}

function close(): void {
  clearCloseTimer()
  isPinned.value = false
  isOpen.value = false
}

function togglePinned(): void {
  if (isOpen.value && isPinned.value) {
    close()
    return
  }

  isPinned.value = true
  open()
}

function scheduleClose(): void {
  if (isPinned.value) return
  clearCloseTimer()
  closeTimer = setTimeout(close, 180)
}

function containsTarget(target: EventTarget | null): boolean {
  return target instanceof Node
    && Boolean(rootRef.value?.contains(target) || popoverRef.value?.contains(target))
}

function handleFocusOut(event: FocusEvent): void {
  if (containsTarget(event.relatedTarget) || isPinned.value) return
  close()
}

function handleDocumentPointerDown(event: PointerEvent): void {
  if (containsTarget(event.target)) return
  close()
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape' && isOpen.value) close()
}

function handleResize(): void {
  if (isOpen.value) void positionPopover()
}

onMounted(() => {
  document.addEventListener('pointerdown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', handleResize, { passive: true })
  window.addEventListener('scroll', handleResize, true)
})

onBeforeUnmount(() => {
  clearCloseTimer()
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('scroll', handleResize, true)
})
</script>

<template>
  <div
    ref="rootRef"
    class="customer-support"
    @mouseenter="open"
    @mouseleave="scheduleClose"
    @focusin="open"
    @focusout="handleFocusOut"
  >
    <button
      type="button"
      class="customer-support__trigger"
      :aria-label="t('nav.customerService')"
      :title="t('nav.customerService')"
      aria-haspopup="true"
      aria-controls="customer-support-popover"
      :aria-expanded="isOpen"
      data-customer-support-trigger
      @click="togglePinned"
    >
      <Icon name="userPlus" size="sm" />
      <span>{{ t('nav.customerService') }}</span>
    </button>

    <Teleport to="body">
      <Transition name="customer-support-popover">
        <section
          v-if="hasOpened"
          v-show="isOpen"
          id="customer-support-popover"
          ref="popoverRef"
          class="customer-support__popover"
          :style="popoverStyle"
          role="group"
          :aria-label="t('nav.customerSupportTitle')"
          :data-customer-support-pinned="isPinned ? 'true' : 'false'"
          data-customer-support-popover
          @mouseenter="clearCloseTimer"
          @mouseleave="scheduleClose"
          @focusout="handleFocusOut"
        >
        <header class="customer-support__header">
          <div>
            <strong>{{ t('nav.customerSupportTitle') }}</strong>
            <span>QQ / {{ t('nav.customerSupportWechat') }}</span>
          </div>
          <button
            type="button"
            class="customer-support__close"
            :aria-label="t('common.close')"
            :title="t('common.close')"
            @click="close"
          >
            <Icon name="x" size="sm" />
          </button>
        </header>

        <div class="customer-support__grid">
          <a
            class="customer-support__card"
            :href="customerSupportContacts.qq.href"
            target="_blank"
            rel="noopener noreferrer"
          >
            <img
              :src="customerSupportContacts.qq.qrSrc"
              :alt="t('nav.customerSupportQqQrAlt')"
              width="152"
              height="152"
              loading="eager"
              decoding="async"
            >
            <strong>{{ t('nav.customerSupportQq') }}</strong>
            <span>{{ customerSupportContacts.qq.account }}</span>
          </a>

          <div class="customer-support__card">
            <img
              :src="customerSupportContacts.wechat.qrSrc"
              :alt="t('nav.customerSupportWechatQrAlt')"
              width="152"
              height="152"
              loading="eager"
              decoding="async"
            >
            <strong>{{ t('nav.customerSupportWechat') }}</strong>
            <span>{{ customerSupportContacts.wechat.account }}</span>
          </div>
        </div>
        <p>{{ t('nav.customerSupportScanHint') }}</p>
        </section>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.customer-support {
  position: relative;
  display: inline-flex;
}

.customer-support__trigger {
  display: inline-flex;
  min-width: 2.25rem;
  min-height: 2.25rem;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  border: 1px solid #0a7f90;
  border-left-width: 3px;
  border-radius: 6px;
  padding: 0 0.7rem;
  color: #075f6c;
  background: rgba(10, 127, 144, 0.12);
  box-shadow: inset 0 0 0 1px rgba(108, 230, 241, 0.09), 0 0 14px rgba(10, 127, 144, 0.1);
  font: inherit;
  font-size: 0.8125rem;
  font-weight: 700;
  white-space: nowrap;
  cursor: pointer;
  transition: color 0.18s ease, background-color 0.18s ease, box-shadow 0.18s ease;
}

.customer-support__trigger:hover,
.customer-support__trigger:focus-visible,
.customer-support__trigger[aria-expanded='true'] {
  color: #087582;
  background: rgba(33, 185, 200, 0.16);
  box-shadow: inset 0 0 0 1px rgba(108, 230, 241, 0.16), 0 0 18px rgba(33, 185, 200, 0.18);
}

.customer-support__trigger:focus-visible,
.customer-support__close:focus-visible,
.customer-support__card:focus-visible {
  outline: 2px solid rgba(33, 185, 200, 0.52);
  outline-offset: 2px;
}

.customer-support__popover {
  position: fixed;
  z-index: 1000;
  max-height: calc(100vh - 24px);
  overflow-y: auto;
  padding: 0.75rem;
  border: 1px solid rgba(10, 127, 144, 0.24);
  border-radius: 8px;
  color: #1a211e;
  background: rgba(250, 251, 248, 0.98);
  box-shadow: 0 20px 48px rgba(20, 29, 26, 0.24), inset 0 1px rgba(255, 255, 255, 0.82);
  backdrop-filter: blur(16px) saturate(130%);
}

.customer-support__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.625rem;
}

.customer-support__header > div {
  display: grid;
  gap: 0.125rem;
}

.customer-support__header strong {
  font-size: 0.875rem;
}

.customer-support__header span,
.customer-support__popover > p {
  color: rgba(35, 43, 39, 0.62);
  font-size: 0.6875rem;
}

.customer-support__close {
  display: grid;
  width: 1.75rem;
  height: 1.75rem;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid rgba(35, 43, 39, 0.12);
  border-radius: 6px;
  color: #3f4844;
  background: rgba(255, 255, 255, 0.72);
  cursor: pointer;
}

.customer-support__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.625rem;
}

.customer-support__card {
  display: grid;
  min-width: 0;
  justify-items: center;
  border: 1px solid rgba(35, 43, 39, 0.1);
  border-radius: 8px;
  padding: 0.625rem;
  color: #1a211e;
  background: rgba(255, 255, 255, 0.74);
  text-decoration: none;
}

.customer-support__card:hover {
  border-color: rgba(10, 127, 144, 0.42);
  background: #fff;
}

.customer-support__card img {
  display: block;
  width: min(100%, 152px);
  aspect-ratio: 1;
  border-radius: 4px;
  object-fit: contain;
}

.customer-support__card strong {
  margin-top: 0.45rem;
  font-size: 0.75rem;
}

.customer-support__card span {
  margin-top: 0.125rem;
  color: rgba(35, 43, 39, 0.66);
  font-size: 0.6875rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.customer-support__popover > p {
  margin: 0.55rem 0 0;
  text-align: center;
}

:global(.dark) .customer-support__trigger {
  color: #69d0da;
  background: rgba(10, 127, 144, 0.18);
}

:global(.dark) .customer-support__popover {
  border-color: rgba(85, 214, 226, 0.22);
  color: #edf8f6;
  background: rgba(7, 21, 28, 0.98);
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.42), inset 0 1px rgba(255, 255, 255, 0.05);
}

:global(.dark) .customer-support__header span,
:global(.dark) .customer-support__popover > p,
:global(.dark) .customer-support__card span {
  color: #91b8b8;
}

:global(.dark) .customer-support__close,
:global(.dark) .customer-support__card {
  border-color: rgba(129, 213, 219, 0.14);
  color: #edf8f6;
  background: rgba(12, 34, 43, 0.9);
}

:global(.dark) .customer-support__card:hover {
  border-color: rgba(85, 214, 226, 0.38);
  background: #0c2a34;
}

.customer-support-popover-enter-active,
.customer-support-popover-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.customer-support-popover-enter-from,
.customer-support-popover-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

@media (max-width: 420px) {
  .customer-support__popover {
    padding: 0.625rem;
  }

  .customer-support__grid {
    gap: 0.5rem;
  }

  .customer-support__card {
    padding: 0.5rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .customer-support__trigger,
  .customer-support-popover-enter-active,
  .customer-support-popover-leave-active {
    transition: none;
  }
}
</style>
