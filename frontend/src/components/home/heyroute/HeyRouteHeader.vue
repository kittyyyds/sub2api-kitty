<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import { setLocale } from '@/i18n'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['nav']
  siteName: string
  siteLogo: string
  docUrl: string
  activeSection: string
  isDark: boolean
  isAuthenticated: boolean
  dashboardPath: string
  modelPlazaEnabled: boolean
  isHeroActive: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  navigate: [sectionId: string]
  toggleTheme: []
}>()

const { locale } = useI18n()
const mobileOpen = shallowRef(false)
const sectionMenuOpen = shallowRef(false)
const sectionPickerRef = shallowRef<HTMLElement | null>(null)
const sectionTriggerRef = shallowRef<HTMLButtonElement | null>(null)
const mobileMenuRef = shallowRef<HTMLElement | null>(null)
const menuButtonRef = shallowRef<HTMLButtonElement | null>(null)
const activeSectionLabel = computed(() => (
  props.copy.sections.find((section) => section.id === props.activeSection)?.label ?? props.copy.home
))

async function toggleLanguage(): Promise<void> {
  await setLocale(locale.value.startsWith('zh') ? 'en' : 'zh')
}

function navigate(sectionId: string): void {
  emit('navigate', sectionId)
  mobileOpen.value = false
  sectionMenuOpen.value = false
}

function handleWindowPointerDown(event: PointerEvent): void {
  const target = event.target as Node
  if (sectionMenuOpen.value && !sectionPickerRef.value?.contains(target)) {
    sectionMenuOpen.value = false
  }
  if (
    mobileOpen.value
    && !mobileMenuRef.value?.contains(target)
    && !menuButtonRef.value?.contains(target)
  ) {
    mobileOpen.value = false
  }
}

function handleWindowKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  const focused = document.activeElement
  if (sectionMenuOpen.value) {
    sectionMenuOpen.value = false
    if (sectionPickerRef.value?.contains(focused)) sectionTriggerRef.value?.focus()
  }
  if (mobileOpen.value) {
    mobileOpen.value = false
    if (mobileMenuRef.value?.contains(focused)) menuButtonRef.value?.focus()
  }
}

onMounted(() => {
  window.addEventListener('pointerdown', handleWindowPointerDown, { passive: true })
  window.addEventListener('keydown', handleWindowKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', handleWindowPointerDown)
  window.removeEventListener('keydown', handleWindowKeydown)
})
</script>

<template>
  <header class="hr-header" :class="{ 'is-over-hero': isHeroActive }">
    <button class="hr-brand" type="button" @click="navigate('hr-hero')">
      <img :src="siteLogo || '/logo.svg'" :alt="siteName" />
      <strong>{{ siteName }}</strong>
    </button>

    <nav class="hr-nav-capsule" :aria-label="copy.primary">
      <button
        type="button"
        class="hr-icon-control hr-language-control"
        :aria-label="copy.switchLanguage"
        :title="copy.switchLanguage"
        @click="toggleLanguage"
      >
        {{ locale.startsWith('zh') ? '中' : 'EN' }}
      </button>
      <button
        type="button"
        class="hr-icon-control"
        :aria-label="copy.switchTheme"
        :title="copy.switchTheme"
        @click="emit('toggleTheme')"
      >
        <Icon :name="isDark ? 'moon' : 'sun'" size="sm" />
      </button>
      <div ref="sectionPickerRef" class="hr-section-picker">
        <button
          ref="sectionTriggerRef"
          type="button"
          class="hr-section-trigger"
          aria-haspopup="menu"
          :aria-expanded="sectionMenuOpen"
          @click="sectionMenuOpen = !sectionMenuOpen"
        >
          <span>{{ activeSectionLabel }}</span>
          <Icon :name="sectionMenuOpen ? 'chevronUp' : 'chevronDown'" size="xs" />
        </button>
        <Transition name="hr-section-menu">
          <div v-if="sectionMenuOpen" class="hr-section-menu" role="menu">
            <button
              v-for="section in copy.sections"
              :key="section.id"
              type="button"
              role="menuitem"
              :class="{ active: section.id === activeSection }"
              @click="navigate(section.id)"
            >
              {{ section.label }}
            </button>
          </div>
        </Transition>
      </div>
      <router-link :to="modelPlazaEnabled ? '/model-plaza' : '/login'" class="hr-nav-link">
        {{ copy.models }}
      </router-link>
      <router-link to="/guide" class="hr-nav-link">{{ locale.startsWith('zh') ? '使用教程' : 'Guide' }}</router-link>
      <a
        v-if="docUrl"
        :href="docUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="hr-nav-link hr-doc-link"
      >
        {{ copy.docs }}
      </a>
      <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="hr-login-link">
        <span>{{ isAuthenticated ? copy.dashboard : copy.login }}</span>
        <Icon name="login" size="sm" />
      </router-link>
      <button
        ref="menuButtonRef"
        type="button"
        class="hr-icon-control hr-menu-control"
        :aria-label="mobileOpen ? copy.closeMenu : copy.menu"
        :aria-expanded="mobileOpen"
        @click="mobileOpen = !mobileOpen"
      >
        <Icon :name="mobileOpen ? 'x' : 'menu'" size="md" />
      </button>
    </nav>

    <Transition name="hr-mobile-menu">
      <nav v-if="mobileOpen" ref="mobileMenuRef" class="hr-mobile-menu" :aria-label="copy.primary">
        <button
          v-for="section in copy.sections"
          :key="section.id"
          type="button"
          :class="{ active: section.id === activeSection }"
          @click="navigate(section.id)"
        >
          {{ section.label }}
        </button>
        <router-link :to="modelPlazaEnabled ? '/model-plaza' : '/login'">{{ copy.models }}</router-link>
        <router-link to="/guide" @click="mobileOpen = false">{{ locale.startsWith('zh') ? '使用教程' : 'Guide' }}</router-link>
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ copy.docs }}</a>
      </nav>
    </Transition>
  </header>
</template>

<style scoped>
.hr-header {
  position: fixed;
  top: 10px;
  right: 40px;
  left: 40px;
  z-index: 80;
  display: flex;
  min-height: 62px;
  align-items: center;
  justify-content: space-between;
  pointer-events: none;
}

.hr-brand,
.hr-nav-capsule,
.hr-mobile-menu {
  pointer-events: auto;
}

.hr-brand {
  display: inline-flex;
  min-width: 0;
  min-height: 44px;
  align-items: center;
  gap: 10px;
  border: 0;
  color: var(--hr-ink);
  background: transparent;
  cursor: pointer;
}

.hr-brand img {
  width: 38px;
  height: 38px;
  flex: 0 0 auto;
  border-radius: 0;
  filter: drop-shadow(0 7px 10px rgba(39, 25, 19, 0.2));
  object-fit: contain;
  transition: transform 360ms cubic-bezier(0.23, 1, 0.32, 1);
}

.hr-brand:hover img {
  transform: rotate(-3deg) scale(1.055);
}

.hr-brand strong {
  max-width: 220px;
  overflow: hidden;
  font-size: 19px;
  font-weight: 750;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (prefers-reduced-motion: reduce) {
  .hr-brand img {
    transition: none;
  }

  .hr-brand:hover img {
    transform: none;
  }
}

.is-over-hero .hr-brand {
  color: var(--hr-hero-ink);
}

.hr-nav-capsule {
  display: flex;
  align-items: center;
  min-height: 40px;
  overflow: visible;
  border: 1px solid var(--hr-glass-border);
  border-radius: 999px;
  background: var(--hr-nav-bg);
  box-shadow: 0 14px 34px var(--hr-shadow);
  backdrop-filter: blur(18px) saturate(145%);
}

.hr-icon-control,
.hr-nav-link,
.hr-section-picker,
.hr-login-link {
  min-height: 38px;
  border-left: 1px solid var(--hr-line);
}

.hr-icon-control {
  display: inline-grid;
  width: 42px;
  place-items: center;
  border-top: 0;
  border-right: 0;
  border-bottom: 0;
  color: var(--hr-ink);
  background: transparent;
  cursor: pointer;
}

.hr-language-control {
  width: 46px;
  border-left: 0;
  font-size: 14px;
  font-weight: 700;
}

.hr-nav-link,
.hr-login-link {
  display: inline-flex;
  align-items: center;
  padding: 0 14px;
  color: var(--hr-ink);
  font-size: 13px;
  font-weight: 520;
  text-decoration: none;
  white-space: nowrap;
}

.hr-section-picker {
  position: relative;
  display: inline-flex;
  min-width: 112px;
  align-items: center;
}

.hr-section-trigger {
  display: flex;
  width: 100%;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 0;
  outline: 0;
  padding: 0 34px 0 14px;
  color: var(--hr-ink);
  background: transparent;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}

.hr-section-trigger svg {
  position: absolute;
  right: 12px;
  pointer-events: none;
}

.hr-section-menu {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  z-index: 10;
  display: grid;
  width: 184px;
  padding: 7px;
  border: 1px solid rgba(255, 255, 255, 0.68);
  border-radius: 16px;
  background: rgba(247, 244, 236, 0.82);
  box-shadow: 0 24px 54px rgba(56, 40, 34, 0.2), inset 0 0 0 1px rgba(255, 255, 255, 0.24);
  backdrop-filter: blur(22px) saturate(145%);
}

.hr-section-menu button {
  display: flex;
  width: 100%;
  min-height: 36px;
  align-items: center;
  border: 0;
  border-radius: 9px;
  padding: 0 10px;
  color: #272724;
  background: transparent;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.hr-section-menu button:hover,
.hr-section-menu button.active {
  background: rgba(255, 255, 255, 0.64);
}

.hr-section-menu-enter-active,
.hr-section-menu-leave-active {
  transition: opacity 190ms ease, transform 240ms cubic-bezier(.16, 1, .3, 1);
  transform-origin: top center;
}

.hr-section-menu-enter-from,
.hr-section-menu-leave-to {
  opacity: 0;
  transform: translateY(-7px) scale(0.97);
}

.hr-login-link {
  min-height: 34px;
  gap: 8px;
  margin: 2px 3px;
  border-left: 0;
  border-radius: 999px;
  color: #f4f5f2;
  background: #1c211f;
  box-shadow: 0 8px 18px rgba(33, 24, 20, 0.2);
}

.hr-menu-control {
  display: none;
}

.hr-icon-control:hover,
.hr-nav-link:hover,
.hr-section-trigger:hover {
  background: var(--hr-hover);
}

.hr-brand:focus-visible,
.hr-icon-control:focus-visible,
.hr-nav-link:focus-visible,
.hr-login-link:focus-visible,
.hr-section-trigger:focus-visible,
.hr-section-menu button:focus-visible,
.hr-mobile-menu a:focus-visible,
.hr-mobile-menu button:focus-visible {
  outline: 2px solid var(--hr-focus);
  outline-offset: 2px;
}

.hr-mobile-menu {
  position: absolute;
  top: 58px;
  right: 0;
  display: none;
  width: min(260px, calc(100vw - 40px));
  padding: 8px;
  border: 1px solid var(--hr-glass-border);
  border-radius: 8px;
  background: var(--hr-panel);
  box-shadow: 0 20px 50px var(--hr-shadow);
}

.hr-mobile-menu button,
.hr-mobile-menu a {
  display: flex;
  width: 100%;
  min-height: 44px;
  align-items: center;
  border: 0;
  border-radius: 6px;
  padding: 0 13px;
  color: var(--hr-ink);
  background: transparent;
  font-size: 14px;
  text-align: left;
  text-decoration: none;
}

.hr-mobile-menu button.active,
.hr-mobile-menu button:hover,
.hr-mobile-menu a:hover {
  background: var(--hr-hover);
}

.hr-mobile-menu-enter-active,
.hr-mobile-menu-leave-active {
  transition: opacity 180ms ease, transform 180ms cubic-bezier(.16, 1, .3, 1);
}

.hr-mobile-menu-enter-from,
.hr-mobile-menu-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

@media (max-width: 820px) {
  .hr-header {
    top: 10px;
    right: 20px;
    left: 20px;
    min-height: 54px;
  }

  .hr-brand strong,
  .hr-section-picker,
  .hr-nav-link {
    display: none;
  }

  .hr-brand img {
    width: 36px;
    height: 36px;
  }

  .hr-nav-capsule {
    min-height: 46px;
  }

  .hr-icon-control {
    width: 44px;
    min-height: 44px;
  }

  .hr-language-control {
    width: 44px;
  }

  .hr-login-link {
    min-height: 40px;
    padding: 0 14px;
  }

  .hr-menu-control,
  .hr-mobile-menu {
    display: grid;
  }
}

@media (max-width: 360px) {
  .hr-header {
    right: 14px;
    left: 14px;
  }

  .hr-login-link span {
    display: none;
  }

  .hr-login-link {
    width: 43px;
    justify-content: center;
    padding: 0;
  }
}
</style>
