<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, shallowRef, watch } from 'vue'

import Icon from '@/components/icons/Icon.vue'

export type HeyRouteContactIcon = 'chat' | 'chatBubble' | 'paperAirplane'

export interface HeyRouteContactPlatform {
  id: string
  label: string
  icon: HeyRouteContactIcon
  title: string
  description: string
  valueLabel: string
  value: string
  copyLabel?: string
  externalLabel?: string
  externalUrl?: string
  qrSrc: string
  qrAlt: string
  qrBackground: string
}

interface Props {
  open: boolean
  dialogId: string
  title: string
  description: string
  closeLabel: string
  platformLabel: string
  platforms: readonly HeyRouteContactPlatform[]
  copiedLabel: string
  copyFailedLabel: string
  scanLabel: string
  notice: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  close: []
}>()

type CopyStatus = 'copied' | 'error' | 'idle'

const activePlatformId = shallowRef('')
const copyStatus = shallowRef<CopyStatus>('idle')
const closeButtonRef = shallowRef<HTMLButtonElement | null>(null)
const titleId = computed(() => `hr-${props.dialogId}-dialog-title`)
const activePlatform = computed(() => (
  props.platforms.find((platform) => platform.id === activePlatformId.value)
  ?? props.platforms[0]
))
let resetCopyTimer: ReturnType<typeof setTimeout> | undefined
let savedBodyOverflow = ''

function legacyCopy(value: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = value
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

function resetCopyStatus(): void {
  if (resetCopyTimer) clearTimeout(resetCopyTimer)
  copyStatus.value = 'idle'
}

function selectPlatform(platformId: string): void {
  activePlatformId.value = platformId
  resetCopyStatus()
}

async function copyValue(): Promise<void> {
  const value = activePlatform.value?.value
  if (!value) return
  resetCopyStatus()

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else if (!legacyCopy(value)) {
      throw new Error('Clipboard unavailable')
    }
    copyStatus.value = 'copied'
  } catch {
    copyStatus.value = 'error'
  }

  resetCopyTimer = setTimeout(() => {
    copyStatus.value = 'idle'
  }, 1800)
}

function close(): void {
  emit('close')
}

function handleKeydown(event: KeyboardEvent): void {
  if (props.open && event.key === 'Escape') close()
}

watch(
  () => props.platforms,
  (platforms) => {
    if (!platforms.some((platform) => platform.id === activePlatformId.value)) {
      activePlatformId.value = platforms[0]?.id ?? ''
    }
  },
  { immediate: true },
)

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      activePlatformId.value = props.platforms[0]?.id ?? ''
      resetCopyStatus()
      savedBodyOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      await nextTick()
      closeButtonRef.value?.focus()
      return
    }

    document.body.style.overflow = savedBodyOverflow
  },
)

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (props.open) document.body.style.overflow = savedBodyOverflow
  if (resetCopyTimer) clearTimeout(resetCopyTimer)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="hr-contact-dialog">
      <div
        v-if="open"
        class="hr-contact-overlay"
        role="presentation"
        @pointerdown.self="close"
      >
        <section
          class="hr-contact-dialog"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
        >
          <div class="hr-contact-ambient" aria-hidden="true" />

          <header class="hr-contact-header">
            <div>
              <h2 :id="titleId">{{ title }}</h2>
              <p>{{ description }}</p>
            </div>
            <button
              ref="closeButtonRef"
              type="button"
              class="hr-contact-close"
              :aria-label="closeLabel"
              :title="closeLabel"
              @click="close"
            >
              <Icon name="x" size="md" />
            </button>
          </header>

          <div class="hr-contact-tabs" role="tablist" :aria-label="platformLabel">
            <button
              v-for="platform in platforms"
              :id="`hr-${dialogId}-${platform.id}-tab`"
              :key="platform.id"
              type="button"
              role="tab"
              :aria-selected="activePlatformId === platform.id"
              :aria-controls="`hr-${dialogId}-${platform.id}-panel`"
              @click="selectPlatform(platform.id)"
            >
              <Icon :name="platform.icon" size="sm" />
              <span>{{ platform.label }}</span>
            </button>
          </div>

          <Transition name="hr-contact-panel" mode="out-in">
            <div
              v-if="activePlatform"
              :id="`hr-${dialogId}-${activePlatform.id}-panel`"
              :key="activePlatform.id"
              class="hr-contact-panel"
              role="tabpanel"
              :aria-labelledby="`hr-${dialogId}-${activePlatform.id}-tab`"
            >
              <div class="hr-contact-copy">
                <span class="hr-contact-platform-icon">
                  <Icon :name="activePlatform.icon" size="md" />
                </span>
                <h3>{{ activePlatform.title }}</h3>
                <p>{{ activePlatform.description }}</p>
                <dl>
                  <div>
                    <dt>{{ activePlatform.valueLabel }}</dt>
                    <dd>{{ activePlatform.value }}</dd>
                  </div>
                </dl>
                <div class="hr-contact-actions">
                  <button
                    v-if="activePlatform.copyLabel"
                    type="button"
                    class="hr-contact-action"
                    @click="copyValue"
                  >
                    <Icon :name="copyStatus === 'copied' ? 'check' : 'copy'" size="sm" />
                    <span aria-live="polite">
                      {{ copyStatus === 'copied' ? copiedLabel : copyStatus === 'error' ? copyFailedLabel : activePlatform.copyLabel }}
                    </span>
                  </button>
                  <a
                    v-if="activePlatform.externalUrl && activePlatform.externalLabel"
                    class="hr-contact-action"
                    :class="{ 'hr-contact-action-secondary': activePlatform.copyLabel }"
                    :href="activePlatform.externalUrl"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Icon name="externalLink" size="sm" />
                    <span>{{ activePlatform.externalLabel }}</span>
                  </a>
                </div>
              </div>

              <figure class="hr-contact-qr-card">
                <img
                  :src="activePlatform.qrSrc"
                  :alt="activePlatform.qrAlt"
                  :style="{ background: activePlatform.qrBackground }"
                />
                <figcaption>{{ scanLabel }}</figcaption>
              </figure>
            </div>
          </Transition>

          <p class="hr-contact-notice">{{ notice }}</p>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.hr-contact-overlay {
  position: fixed;
  z-index: 180;
  inset: 0;
  display: grid;
  overflow-y: auto;
  place-items: center;
  padding: 28px;
  background: rgba(19, 24, 22, 0.46);
  backdrop-filter: blur(10px) saturate(125%);
}

.hr-contact-dialog {
  position: relative;
  width: min(650px, 100%);
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 28px;
  padding: 30px;
  color: #1a211e;
  background: rgba(247, 247, 242, 0.94);
  box-shadow: 0 28px 92px rgba(11, 15, 13, 0.34), inset 0 1px rgba(255, 255, 255, 0.62);
  isolation: isolate;
}

.hr-contact-dialog > :not(.hr-contact-ambient) {
  position: relative;
  z-index: 1;
}

.hr-contact-ambient {
  position: absolute;
  z-index: 0;
  inset: -24% -12% auto;
  height: 290px;
  opacity: 0.58;
  background:
    radial-gradient(circle at 13% 38%, rgba(255, 143, 121, 0.76), transparent 31%),
    radial-gradient(circle at 49% 15%, rgba(244, 215, 93, 0.54), transparent 30%),
    radial-gradient(circle at 82% 42%, rgba(81, 200, 184, 0.62), transparent 35%);
  filter: blur(34px);
  pointer-events: none;
}

.hr-contact-header {
  display: flex;
  min-height: 68px;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
}

.hr-contact-header h2,
.hr-contact-header p,
.hr-contact-copy h3,
.hr-contact-copy p,
.hr-contact-notice {
  margin: 0;
}

.hr-contact-header h2 {
  font-size: 32px;
  font-weight: 760;
  line-height: 1.08;
}

.hr-contact-header p {
  max-width: 470px;
  margin-top: 8px;
  color: rgba(35, 43, 39, 0.67);
  font-size: 12px;
  font-weight: 550;
  line-height: 1.6;
}

.hr-contact-close {
  display: grid;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid rgba(33, 42, 38, 0.12);
  border-radius: 50%;
  color: #404744;
  background: rgba(255, 250, 248, 0.74);
  box-shadow: 0 6px 16px rgba(42, 31, 27, 0.09);
  cursor: pointer;
}

.hr-contact-close:hover {
  color: #171d1a;
  background: #fff;
  transform: rotate(7deg);
}

.hr-contact-tabs {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
  margin-top: 24px;
  border: 1px solid rgba(255, 255, 255, 0.78);
  border-radius: 14px;
  padding: 4px;
  background: rgba(255, 255, 255, 0.55);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.64);
}

.hr-contact-tabs button {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 0;
  border-radius: 10px;
  color: rgba(44, 51, 48, 0.64);
  background: transparent;
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
}

.hr-contact-tabs button[aria-selected='true'] {
  color: #f8faf7;
  background: #1a1f1c;
  box-shadow: 0 7px 16px rgba(24, 29, 26, 0.16);
}

.hr-contact-panel {
  display: grid;
  min-height: 282px;
  grid-template-columns: minmax(0, 1fr) 248px;
  align-items: center;
  gap: 36px;
  margin-top: 14px;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 22px;
  padding: 25px;
  background: rgba(252, 252, 248, 0.64);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.6);
}

.hr-contact-copy {
  min-width: 0;
}

.hr-contact-platform-icon {
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border: 1px solid rgba(37, 47, 43, 0.12);
  border-radius: 12px;
  color: #176f69;
  background: rgba(255, 255, 255, 0.58);
}

.hr-contact-copy h3 {
  margin-top: 17px;
  font-size: 21px;
  font-weight: 760;
  line-height: 1.2;
}

.hr-contact-copy p {
  max-width: 250px;
  margin-top: 8px;
  color: rgba(36, 44, 40, 0.68);
  font-size: 12px;
  font-weight: 540;
  line-height: 1.65;
}

.hr-contact-copy dl {
  margin: 21px 0 0;
}

.hr-contact-copy dt {
  color: rgba(37, 45, 41, 0.58);
  font-size: 10px;
  font-weight: 700;
}

.hr-contact-copy dd {
  overflow: hidden;
  margin: 5px 0 0;
  color: #171d1a;
  font-size: 15px;
  font-weight: 760;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hr-contact-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 22px;
}

.hr-contact-action {
  display: inline-flex;
  min-height: 39px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px solid rgba(20, 25, 22, 0.9);
  border-radius: 11px;
  padding: 0 13px;
  color: #f6f8f5;
  background: #171c19;
  box-shadow: 0 10px 20px rgba(26, 28, 25, 0.16);
  font: inherit;
  font-size: 11px;
  font-weight: 740;
  text-decoration: none;
  cursor: pointer;
}

.hr-contact-action-secondary {
  border-color: rgba(29, 37, 33, 0.17);
  color: #252d29;
  background: rgba(255, 255, 255, 0.72);
  box-shadow: 0 8px 18px rgba(42, 35, 31, 0.08);
}

.hr-contact-action:hover {
  background: #050806;
  box-shadow: 0 14px 28px rgba(26, 28, 25, 0.23);
  transform: translateY(-1px);
}

.hr-contact-action-secondary:hover {
  color: #171d1a;
  background: #fff;
}

.hr-contact-qr-card {
  display: grid;
  width: 100%;
  margin: 0;
  overflow: hidden;
  justify-self: end;
  border: 1px solid rgba(37, 46, 42, 0.1);
  border-radius: 18px;
  padding: 12px 12px 10px;
  background: rgba(255, 255, 255, 0.77);
  box-shadow: 0 18px 34px rgba(45, 36, 31, 0.12), inset 0 1px rgba(255, 255, 255, 0.68);
}

.hr-contact-qr-card img {
  display: block;
  width: 100%;
  aspect-ratio: 1;
  border-radius: 10px;
  object-fit: contain;
}

.hr-contact-qr-card figcaption {
  margin-top: 8px;
  color: rgba(35, 44, 40, 0.68);
  font-size: 10px;
  font-weight: 650;
  text-align: center;
}

.hr-contact-notice {
  margin-top: 15px;
  color: rgba(40, 48, 44, 0.61);
  font-size: 10px;
  font-weight: 520;
  line-height: 1.65;
  text-align: center;
}

.hr-contact-close:focus-visible,
.hr-contact-tabs button:focus-visible,
.hr-contact-action:focus-visible {
  outline: 2px solid #176f69;
  outline-offset: 3px;
}

.hr-contact-dialog-enter-active,
.hr-contact-dialog-leave-active {
  transition: opacity 220ms ease;
}

.hr-contact-dialog-enter-active .hr-contact-dialog,
.hr-contact-dialog-leave-active .hr-contact-dialog {
  transition: opacity 260ms cubic-bezier(.16, 1, .3, 1), transform 260ms cubic-bezier(.16, 1, .3, 1);
}

.hr-contact-dialog-enter-from,
.hr-contact-dialog-leave-to {
  opacity: 0;
}

.hr-contact-dialog-enter-from .hr-contact-dialog,
.hr-contact-dialog-leave-to .hr-contact-dialog {
  opacity: 0;
  transform: translateY(18px) scale(0.97);
}

.hr-contact-panel-enter-active,
.hr-contact-panel-leave-active {
  transition: opacity 150ms ease;
}

.hr-contact-panel-enter-from,
.hr-contact-panel-leave-to {
  opacity: 0;
}

@media (max-width: 620px) {
  .hr-contact-overlay {
    align-items: start;
    padding: 16px;
  }

  .hr-contact-dialog {
    margin: 22px 0;
    border-radius: 22px;
    padding: 22px 17px 18px;
  }

  .hr-contact-header {
    min-height: 0;
    gap: 14px;
  }

  .hr-contact-header h2 {
    font-size: 26px;
  }

  .hr-contact-header p {
    font-size: 11px;
  }

  .hr-contact-close {
    width: 38px;
    height: 38px;
  }

  .hr-contact-tabs {
    margin-top: 19px;
  }

  .hr-contact-panel {
    min-height: 0;
    grid-template-columns: minmax(0, 1fr);
    gap: 24px;
    padding: 21px;
  }

  .hr-contact-copy p {
    max-width: none;
  }

  .hr-contact-qr-card {
    width: min(238px, 100%);
    justify-self: center;
  }

  .hr-contact-notice {
    text-align: left;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hr-contact-close:hover,
  .hr-contact-action:hover {
    transform: none;
  }

  .hr-contact-dialog-enter-active,
  .hr-contact-dialog-leave-active,
  .hr-contact-dialog-enter-active .hr-contact-dialog,
  .hr-contact-dialog-leave-active .hr-contact-dialog,
  .hr-contact-panel-enter-active,
  .hr-contact-panel-leave-active {
    transition: none;
  }
}
</style>
