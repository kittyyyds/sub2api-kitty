<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import HeyRouteGlassCursor from '@/components/home/heyroute/HeyRouteGlassCursor.vue'
import { useTheme } from '@/composables/useTheme'
import { setLocale } from '@/i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

interface Props {
  mode: 'login' | 'register'
}

const props = defineProps<Props>()
const appStore = useAppStore()
const { locale } = useI18n()
const motionReady = shallowRef(false)
const { isDark, toggleTheme } = useTheme()
let motionFrame = 0

const isChinese = computed(() => locale.value.startsWith('zh'))
const siteName = computed(() => appStore.siteName || appStore.cachedPublicSettings?.site_name || 'Kitty奇妙屋')
const siteLogo = computed(() => appStore.brandLogoUrl)
const docUrl = computed(() => sanitizeUrl(
  appStore.docUrl || appStore.cachedPublicSettings?.doc_url || '',
))
const modelPath = computed(() => (
  appStore.cachedPublicSettings?.model_plaza_enabled === true ? '/model-plaza' : '/home'
))
const copy = computed(() => {
  if (isChinese.value) {
    return props.mode === 'login'
      ? {
          title: '欢迎回来',
          titleLines: ['欢迎回来'],
          description: '登录后继续管理 API Key、用量与账户设置。',
          home: '回到主页',
          models: '模型',
          docs: '帮助文档',
          login: '登录',
          register: '注册',
          primary: '账户导航',
          switchLanguage: '切换语言',
          switchTheme: '切换主题',
        }
      : {
          title: `创建你的 ${siteName.value} 账户`,
          titleLines: ['创建你的', siteName.value, '账户'],
          description: '完成邮箱验证后即可进入控制台，创建 API Key 并接入客户端。',
          home: '回到主页',
          models: '模型',
          docs: '帮助文档',
          login: '登录',
          register: '注册',
          primary: '账户导航',
          switchLanguage: '切换语言',
          switchTheme: '切换主题',
        }
  }

  return props.mode === 'login'
    ? {
        title: 'Welcome back',
        titleLines: ['Welcome', 'Back'],
        description: 'Sign in to manage API keys, model usage, and account settings.',
        home: 'Back home',
        models: 'Models',
        docs: 'Docs',
        login: 'Sign in',
        register: 'Register',
        primary: 'Account navigation',
        switchLanguage: 'Switch language',
        switchTheme: 'Switch theme',
      }
    : {
        title: `Create your ${siteName.value} account`,
        titleLines: ['Create your', `${siteName.value} account`],
        description: 'Verify your email, enter the console, and create an API key for your existing client.',
        home: 'Back home',
        models: 'Models',
        docs: 'Docs',
        login: 'Sign in',
        register: 'Register',
        primary: 'Account navigation',
        switchLanguage: 'Switch language',
        switchTheme: 'Switch theme',
      }
})

async function toggleLanguage(): Promise<void> {
  await setLocale(isChinese.value ? 'en' : 'zh')
}

function finishHeadingAnimation(event: AnimationEvent): void {
  if (event.animationName.includes('hr-auth-heading-mask')) {
    (event.currentTarget as HTMLElement).style.clipPath = 'none'
  }
}

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }

  motionFrame = window.requestAnimationFrame(() => {
    motionReady.value = true
  })
})

onBeforeUnmount(() => {
  window.cancelAnimationFrame(motionFrame)
})
</script>

<template>
  <div
    class="heyroute-preview hr-auth-layout"
    :class="{
      'is-register': mode === 'register',
      'is-motion-ready': motionReady,
    }"
    :data-theme="isDark ? 'dark' : 'light'"
    data-hr-auth
  >
    <header class="hr-header hr-auth-header">
      <router-link to="/home" class="hr-auth-brand" :aria-label="`${siteName} ${copy.home}`">
        <img :src="siteLogo || '/logo.svg'" :alt="siteName" />
        <strong>{{ siteName }}</strong>
      </router-link>

      <nav class="hr-auth-nav" :aria-label="copy.primary">
        <button type="button" :aria-label="copy.switchLanguage" :title="copy.switchLanguage" @click="toggleLanguage">
          {{ isChinese ? '中' : 'EN' }}
        </button>
        <button type="button" :aria-label="copy.switchTheme" :title="copy.switchTheme" @click="toggleTheme">
          <Icon :name="isDark ? 'moon' : 'sun'" size="sm" />
        </button>
        <router-link class="hr-auth-home" to="/home">{{ copy.home }}</router-link>
        <router-link class="hr-auth-models" :to="modelPath">{{ copy.models }}</router-link>
        <a v-if="docUrl" class="hr-auth-docs" :href="docUrl" target="_blank" rel="noopener noreferrer">
          {{ copy.docs }}
        </a>
        <div class="hr-auth-account-switch">
          <router-link to="/login" :class="{ active: mode === 'login' }">{{ copy.login }}</router-link>
          <span>/</span>
          <router-link to="/register" :class="{ active: mode === 'register' }">{{ copy.register }}</router-link>
          <Icon name="login" size="sm" />
        </div>
      </nav>
    </header>

    <main class="hr-auth-main">
      <section class="hr-auth-intro" aria-labelledby="hr-auth-title">
        <h1 id="hr-auth-title" @animationend="finishHeadingAnimation">
          <span v-for="line in copy.titleLines" :key="line" class="hr-auth-title-line">{{ line }}</span>
        </h1>
        <p>{{ copy.description }}</p>
      </section>

      <section class="hr-auth-panel" :aria-label="copy.title">
        <slot />
        <footer v-if="$slots.footer" class="hr-auth-panel-footer">
          <slot name="footer" />
        </footer>
      </section>
    </main>

    <HeyRouteGlassCursor />
  </div>
</template>

<style scoped>
.hr-auth-layout {
  --hr-auth-ink: #171b1a;
  --hr-auth-muted: rgba(35, 42, 39, 0.62);
  --hr-auth-line: rgba(41, 48, 44, 0.15);
  --hr-auth-shell: min(1160px, calc(100% - 80px));
  --hr-auth-ease-out: cubic-bezier(0.23, 1, 0.32, 1);
  position: relative;
  min-height: 100dvh;
  overflow-x: clip;
  color: var(--hr-auth-ink);
  background: transparent;
  font-family: 'Montserrat Variable', Montserrat, 'Heiti SC', '黑体-简', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}

.hr-auth-layout.is-register {
  --hr-auth-shell: min(1400px, calc(100% - 80px));
}

.hr-auth-layout[data-theme='dark'] {
  --hr-auth-ink: #f1f4ef;
  --hr-auth-muted: rgba(235, 240, 235, 0.68);
  --hr-auth-line: rgba(238, 243, 239, 0.17);
}

.hr-auth-header {
  position: fixed;
  top: 24px;
  right: 40px;
  left: 40px;
  z-index: 20;
  display: flex;
  min-height: 48px;
  align-items: center;
  justify-content: space-between;
  pointer-events: none;
}

.hr-auth-brand,
.hr-auth-nav {
  pointer-events: auto;
}

.hr-auth-brand {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  gap: 10px;
  color: #f5f7f3;
  text-decoration: none;
}

.hr-auth-brand img {
  width: 38px;
  height: 38px;
  border-radius: 0;
  filter: drop-shadow(0 7px 10px rgba(39, 25, 19, 0.2));
  object-fit: contain;
}

.hr-auth-brand strong {
  max-width: 220px;
  overflow: hidden;
  font-size: 19px;
  font-weight: 750;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hr-auth-nav {
  display: flex;
  min-height: 42px;
  align-items: center;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.62);
  border-radius: 999px;
  background: rgba(245, 243, 234, 0.64);
  box-shadow: 0 16px 34px rgba(57, 42, 34, 0.22);
  backdrop-filter: blur(18px) saturate(145%);
}

.hr-auth-nav > button,
.hr-auth-nav > a {
  display: inline-grid;
  min-width: 44px;
  min-height: 40px;
  place-items: center;
  border: 0;
  border-left: 1px solid rgba(43, 49, 45, 0.1);
  padding: 0 14px;
  color: #1b211f;
  background: transparent;
  font: inherit;
  font-size: 13px;
  font-weight: 560;
  text-decoration: none;
  white-space: nowrap;
  cursor: pointer;
}

.hr-auth-nav > button:first-child {
  border-left: 0;
  font-weight: 720;
}

.hr-auth-nav > button:hover,
.hr-auth-nav > a:hover {
  background: rgba(255, 255, 255, 0.38);
}

.hr-auth-account-switch {
  display: flex;
  min-height: 36px;
  align-items: center;
  gap: 4px;
  margin: 2px 3px;
  border-radius: 999px;
  padding: 0 13px;
  color: #f5f7f3;
  background: #1d211f;
  box-shadow: 0 8px 18px rgba(33, 24, 20, 0.2);
}

.hr-auth-account-switch a,
.hr-auth-account-switch span {
  color: inherit;
  font-size: 12px;
  font-weight: 540;
  text-decoration: none;
  opacity: 0.72;
}

.hr-auth-account-switch a.active,
.hr-auth-account-switch a:hover {
  opacity: 1;
}

.hr-auth-account-switch svg {
  margin-left: 5px;
}

.hr-auth-main {
  position: relative;
  z-index: 4;
  display: grid;
  width: var(--hr-auth-shell);
  min-height: 100dvh;
  grid-template-columns: minmax(0, 1fr) minmax(380px, 520px);
  align-items: center;
  gap: clamp(56px, 9vw, 136px);
  margin: 0 auto;
  padding: 116px 0 56px;
}

.hr-auth-intro {
  max-width: 560px;
}

.hr-auth-intro h1 {
  max-width: 9ch;
  margin: 0;
  clip-path: inset(-0.24em 0 -0.24em 0);
  font-size: clamp(3.4rem, 6.4vw, 6.25rem);
  font-weight: 600;
  letter-spacing: 0;
  line-height: 0.94;
  text-wrap: balance;
}

.hr-auth-title-line {
  display: block;
  white-space: nowrap;
}

.hr-auth-intro p {
  max-width: 34rem;
  margin: 26px 0 0;
  color: var(--hr-auth-muted);
  font-size: 17px;
  font-weight: 500;
  letter-spacing: 0;
  line-height: 1.65;
  text-wrap: pretty;
}

.hr-auth-panel {
  display: flex;
  width: 100%;
  min-height: 420px;
  flex-direction: column;
  justify-self: end;
  border: 1px solid rgba(255, 255, 255, 0.54);
  border-radius: 28px;
  padding: 34px;
  background: rgba(255, 255, 255, 0.3);
  box-shadow: 0 28px 76px rgba(48, 35, 43, 0.18), inset 0 0 0 1px rgba(255, 255, 255, 0.2);
  backdrop-filter: blur(24px) saturate(165%);
  transform-origin: center;
}

.is-register .hr-auth-main {
  grid-template-columns: minmax(0, 1fr) minmax(440px, 560px);
}

.is-register .hr-auth-panel {
  min-height: 0;
}

.is-register .hr-auth-intro {
  max-width: 704px;
}

.is-register .hr-auth-intro h1 {
  max-width: 8ch;
  font-size: clamp(3.1rem, 5vw, 4.8rem);
}

.hr-auth-panel-footer {
  margin-top: auto;
  padding-top: 22px;
  text-align: center;
}

.hr-auth-panel :deep(.input-label) {
  margin-bottom: 8px;
  color: #282d2a;
  font-size: 12px;
  font-weight: 700;
}

.hr-auth-panel :deep(.input) {
  min-height: 50px;
  border: 1px solid rgba(23, 23, 25, 0.14);
  border-radius: 14px;
  color: #1d211f;
  background: rgba(255, 255, 255, 0.32);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.22), 0 1px rgba(255, 255, 255, 0.08);
  font-family: inherit;
  font-size: 13px;
}

.hr-auth-panel :deep(.input:focus) {
  border-color: rgba(23, 23, 25, 0.58);
  background: rgba(255, 255, 255, 0.52);
  box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.38), 0 0 0 4px rgba(23, 23, 25, 0.42);
}

.hr-auth-panel :deep(.btn-primary) {
  min-height: 48px;
  border: 1px solid rgba(16, 16, 18, 0.9);
  border-radius: 13px;
  color: #f5f7f3;
  background: rgba(16, 16, 18, 0.94);
  box-shadow: 0 14px 30px rgba(27, 24, 28, 0.16), inset 0 1px rgba(255, 255, 255, 0.08);
  font-size: 13px;
  font-weight: 750;
}

.hr-auth-panel :deep(.btn-primary:hover:not(:disabled)) {
  background: #050506;
  box-shadow: 0 18px 38px rgba(27, 24, 28, 0.2), inset 0 1px rgba(255, 255, 255, 0.1);
  transform: translateY(-1px);
}

.hr-auth-panel :deep(.btn-secondary) {
  min-height: 48px;
  border-color: rgba(43, 49, 45, 0.16);
  border-radius: 13px;
  background: rgba(250, 244, 235, 0.46);
}

.hr-auth-panel :deep(.btn-primary > svg:not(.animate-spin)) {
  display: none;
}

.hr-auth-panel-footer :deep(p),
.hr-auth-panel-footer :deep(a) {
  color: #333936;
}

.hr-auth-panel-footer :deep(a) {
  font-weight: 720;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.hr-auth-layout[data-theme='dark'] .hr-auth-panel {
  border-color: rgba(235, 241, 237, 0.18);
  background: rgba(31, 32, 38, 0.48);
  box-shadow: 0 28px 76px rgba(0, 0, 0, 0.34), inset 0 0 0 1px rgba(244, 247, 244, 0.055);
}

.hr-auth-layout[data-theme='dark'] .hr-auth-panel :deep(.input-label),
.hr-auth-layout[data-theme='dark'] .hr-auth-panel-footer :deep(p),
.hr-auth-layout[data-theme='dark'] .hr-auth-panel-footer :deep(a) {
  color: #edf1ed;
}

.hr-auth-layout[data-theme='dark'] .hr-auth-panel :deep(.input) {
  border-color: rgba(238, 243, 239, 0.16);
  color: #f1f4ef;
  background: rgba(22, 27, 28, 0.5);
}

@keyframes hr-auth-card-in {
  0% {
    opacity: 0;
    transform: perspective(1200px) rotateY(-42deg) translateZ(-12px);
  }
  64% {
    opacity: 1;
    transform: perspective(1200px) rotateY(3deg) translateZ(0);
  }
  100% {
    opacity: 1;
    transform: perspective(1200px) rotateY(0) translateZ(0);
  }
}

@keyframes hr-auth-heading-mask {
  0% {
    opacity: 0;
    clip-path: inset(0 0 100% 0);
    transform: translateY(22px);
  }
  100% {
    opacity: 1;
    clip-path: inset(-0.24em 0 -0.24em 0);
    transform: translateY(0);
  }
}

@keyframes hr-auth-copy-in {
  0% {
    opacity: 0;
    filter: blur(5px);
    transform: translateY(18px);
  }
  100% {
    opacity: 1;
    filter: blur(0);
    transform: translateY(0);
  }
}

.is-motion-ready .hr-auth-panel {
  animation: hr-auth-card-in 760ms cubic-bezier(0.32, 0.72, 0, 1) both;
}

.is-motion-ready .hr-auth-intro h1 {
  animation: hr-auth-heading-mask 620ms var(--hr-auth-ease-out) 80ms both;
}

.is-motion-ready .hr-auth-intro p {
  animation: hr-auth-copy-in 460ms var(--hr-auth-ease-out) 260ms both;
}

.hr-auth-brand:focus-visible,
.hr-auth-nav button:focus-visible,
.hr-auth-nav a:focus-visible,
.hr-auth-account-switch a:focus-visible {
  outline: 2px solid #176f69;
  outline-offset: 2px;
}

@media (max-width: 980px) {
  .hr-auth-header {
    right: 20px;
    left: 20px;
  }

  .hr-auth-home,
  .hr-auth-models,
  .hr-auth-docs {
    display: none !important;
  }
}

@media (max-width: 900px) {
  .hr-auth-main,
  .is-register .hr-auth-main {
    --hr-auth-shell: min(680px, calc(100% - 40px));
    width: min(calc(100% - 40px), 680px);
    min-height: 100dvh;
    align-content: start;
    align-items: start;
    grid-template-columns: minmax(0, 1fr);
    gap: 32px;
    padding: 120px 0 48px;
  }

  .hr-auth-intro,
  .is-register .hr-auth-intro {
    max-width: 620px;
  }

  .hr-auth-intro h1,
  .is-register .hr-auth-intro h1 {
    max-width: 12ch;
    font-size: clamp(3rem, 9vw, 5rem);
  }

  .hr-auth-intro p {
    margin-top: 18px;
  }

  .hr-auth-panel {
    min-height: 0;
    justify-self: stretch;
  }

  .hr-auth-panel-footer {
    margin-top: 22px;
    padding-top: 0;
  }
}

@media (max-width: 640px) {
  .hr-auth-brand strong {
    display: none;
  }

  .hr-auth-nav {
    min-height: 46px;
  }

  .hr-auth-nav > button {
    min-width: 42px;
    min-height: 44px;
    padding: 0;
  }

  .hr-auth-account-switch {
    min-height: 40px;
    padding: 0 11px;
  }

  .hr-auth-account-switch a,
  .hr-auth-account-switch span {
    font-size: 11px;
  }

  .hr-auth-main,
  .is-register .hr-auth-main {
    width: calc(100% - 32px);
    gap: 24px;
    padding: 96px 0 28px;
  }

  .hr-auth-intro h1,
  .is-register .hr-auth-intro h1 {
    max-width: 11ch;
    font-size: clamp(2.65rem, 11vw, 4rem);
    line-height: 0.96;
  }

  .is-register .hr-auth-intro h1 {
    max-width: none;
    font-size: clamp(2.5rem, 10vw, 3.1rem);
  }

  .hr-auth-intro p {
    margin-top: 14px;
    font-size: 15px;
    line-height: 1.55;
  }

  .hr-auth-panel {
    border-radius: 24px;
    padding: 24px 20px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hr-auth-panel,
  .hr-auth-intro h1,
  .hr-auth-intro p {
    animation: none !important;
    opacity: 1;
    filter: none;
    transform: none;
  }
}
</style>
