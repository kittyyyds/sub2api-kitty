<template>
  <div
    v-if="siteKey"
    class="turnstile-wrapper"
    :data-turnstile-state="loadState"
  >
    <div ref="containerRef" class="turnstile-container"></div>

    <div
      v-if="loadState === 'loading'"
      class="turnstile-status"
      role="status"
    >
      <span class="turnstile-spinner" aria-hidden="true"></span>
      <span>{{ t('common.loading') }}</span>
    </div>

    <div
      v-else-if="loadState === 'error'"
      class="turnstile-status turnstile-status-error"
      role="alert"
    >
      <span>{{ t('auth.turnstileFailed') }}</span>
      <button type="button" class="turnstile-retry" @click="initialize">
        {{ t('common.retry') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'

interface TurnstileRenderOptions {
  sitekey: string
  callback: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
  theme?: 'light' | 'dark' | 'auto'
  size?: 'normal' | 'compact' | 'flexible'
  language?: string
}

interface TurnstileAPI {
  render: (container: HTMLElement, options: TurnstileRenderOptions) => string
  reset: (widgetId?: string) => void
  remove: (widgetId?: string) => void
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
  }
}

type TurnstileLoadState = 'loading' | 'ready' | 'error'

const TURNSTILE_SCRIPT_URL =
  'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
const TURNSTILE_SCRIPT_SELECTOR = 'script[data-kitty-turnstile-api]'
const TURNSTILE_SCRIPT_TIMEOUT_MS = 12_000
const TURNSTILE_RETRY_DELAYS_MS = [500, 1_500]

let turnstileScriptRequest: Promise<void> | null = null

function findTurnstileScript(): HTMLScriptElement | null {
  const managedScript = document.querySelector<HTMLScriptElement>(TURNSTILE_SCRIPT_SELECTOR)
  if (managedScript) return managedScript

  return Array.from(document.scripts).find((script) => {
    try {
      const url = new URL(script.src)
      return (
        url.origin === 'https://challenges.cloudflare.com' &&
        url.pathname === '/turnstile/v0/api.js'
      )
    } catch {
      return false
    }
  }) ?? null
}

function loadTurnstileScript(): Promise<void> {
  if (window.turnstile) return Promise.resolve()
  if (turnstileScriptRequest) return turnstileScriptRequest

  const request = new Promise<void>((resolve, reject) => {
    let settled = false
    const existingScript = findTurnstileScript()
    const script = existingScript ?? document.createElement('script')

    script.dataset.kittyTurnstileApi = 'true'

    const cleanup = () => {
      window.clearInterval(readinessTimer)
      window.clearTimeout(timeoutTimer)
      script.removeEventListener('load', handleLoad)
      script.removeEventListener('error', handleError)
    }

    const finish = (error?: Error) => {
      if (settled) return
      settled = true
      cleanup()
      if (error) reject(error)
      else resolve()
    }

    const checkReadiness = () => {
      if (window.turnstile) finish()
    }

    const handleLoad = () => {
      script.dataset.kittyTurnstileState = 'loaded'
      checkReadiness()
    }

    const handleError = () => {
      script.dataset.kittyTurnstileState = 'error'
      finish(new Error('Failed to load Turnstile script'))
    }

    script.addEventListener('load', handleLoad)
    script.addEventListener('error', handleError)

    const readinessTimer = window.setInterval(checkReadiness, 50)
    const timeoutTimer = window.setTimeout(() => {
      finish(new Error('Timed out while loading Turnstile script'))
    }, TURNSTILE_SCRIPT_TIMEOUT_MS)

    if (!existingScript) {
      script.src = TURNSTILE_SCRIPT_URL
      script.async = true
      script.defer = true
      document.head.appendChild(script)
    }

    checkReadiness()
  })

  let trackedRequest: Promise<void>
  trackedRequest = request.finally(() => {
    if (turnstileScriptRequest === trackedRequest) {
      turnstileScriptRequest = null
    }
  })
  turnstileScriptRequest = trackedRequest
  return trackedRequest
}

function discardFailedTurnstileScript(): void {
  const script = document.querySelector<HTMLScriptElement>(TURNSTILE_SCRIPT_SELECTOR)
  if (!window.turnstile && script) script.remove()
}

const props = withDefaults(
  defineProps<{
    siteKey: string
    theme?: 'light' | 'dark' | 'auto'
    size?: 'normal' | 'compact' | 'flexible'
  }>(),
  {
    theme: 'auto',
    size: 'flexible'
  }
)

const emit = defineEmits<{
  (e: 'verify', token: string): void
  (e: 'expire'): void
  (e: 'error'): void
}>()

const { locale, t } = useI18n()
const containerRef = ref<HTMLElement | null>(null)
const widgetId = ref<string | null>(null)
const scriptLoaded = ref(false)
const loadState = ref<TurnstileLoadState>('loading')
const turnstileLanguage = computed(() => locale.value === 'zh' ? 'zh-CN' : 'en')
let initializationGeneration = 0
let retryTimer: number | null = null
let mounted = false

const renderWidget = () => {
  if (!window.turnstile || !containerRef.value || !props.siteKey) {
    return
  }

  // Remove existing widget if any
  if (widgetId.value) {
    try {
      window.turnstile.remove(widgetId.value)
    } catch {
      // Ignore errors when removing
    }
    widgetId.value = null
  }

  // Clear container
  containerRef.value.innerHTML = ''

  widgetId.value = window.turnstile.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: (token: string) => {
      emit('verify', token)
    },
    'expired-callback': () => {
      emit('expire')
    },
    'error-callback': () => {
      emit('error')
    },
    theme: props.theme,
    size: props.size,
    language: turnstileLanguage.value
  })
}

async function initializeAttempt(generation: number, attempt: number): Promise<void> {
  try {
    await loadTurnstileScript()
    if (!mounted || generation !== initializationGeneration) return

    scriptLoaded.value = true
    renderWidget()
    loadState.value = 'ready'
  } catch (error) {
    if (!mounted || generation !== initializationGeneration) return

    console.error('Failed to initialize Turnstile:', error)
    scriptLoaded.value = false
    discardFailedTurnstileScript()

    const retryDelay = TURNSTILE_RETRY_DELAYS_MS[attempt]
    if (retryDelay !== undefined) {
      retryTimer = window.setTimeout(() => {
        retryTimer = null
        void initializeAttempt(generation, attempt + 1)
      }, retryDelay)
      return
    }

    loadState.value = 'error'
    emit('error')
  }
}

function initialize(): void {
  initializationGeneration += 1
  if (retryTimer !== null) {
    window.clearTimeout(retryTimer)
    retryTimer = null
  }
  loadState.value = 'loading'
  scriptLoaded.value = false
  void initializeAttempt(initializationGeneration, 0)
}

const reset = () => {
  if (window.turnstile && widgetId.value) {
    window.turnstile.reset(widgetId.value)
  }
}

// Expose reset method to parent
defineExpose({ reset })

onMounted(() => {
  mounted = true
  if (props.siteKey) initialize()
})

onUnmounted(() => {
  mounted = false
  initializationGeneration += 1
  if (retryTimer !== null) {
    window.clearTimeout(retryTimer)
    retryTimer = null
  }
  if (window.turnstile && widgetId.value) {
    try {
      window.turnstile.remove(widgetId.value)
    } catch {
      // Ignore errors when removing
    }
  }
})

// Re-render when the site key or application language changes.
watch(
  () => props.siteKey,
  (newKey, oldKey) => {
    if (mounted && newKey && newKey !== oldKey) initialize()
  }
)

watch(
  () => locale.value,
  () => {
    if (props.siteKey && scriptLoaded.value) {
      renderWidget()
    }
  }
)
</script>

<style scoped>
.turnstile-wrapper {
  position: relative;
  width: 100%;
  min-height: 65px;
}

.turnstile-container {
  width: 100%;
  min-height: 65px;
}

.turnstile-wrapper:not([data-turnstile-state='ready']) .turnstile-container {
  opacity: 0;
}

.turnstile-status {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  min-height: 65px;
  border: 1px solid rgb(229 231 235);
  border-radius: 0.5rem;
  background: rgb(249 250 251);
  color: rgb(107 114 128);
  font-size: 0.875rem;
}

:global(.dark) .turnstile-status {
  border-color: rgb(55 65 81);
  background: rgb(17 24 39 / 0.72);
  color: rgb(156 163 175);
}

.turnstile-status-error {
  color: rgb(185 28 28);
}

:global(.dark) .turnstile-status-error {
  color: rgb(248 113 113);
}

.turnstile-spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: turnstile-spin 0.75s linear infinite;
}

.turnstile-retry {
  color: rgb(2 132 199);
  font-weight: 600;
}

.turnstile-retry:hover {
  text-decoration: underline;
}

@keyframes turnstile-spin {
  to {
    transform: rotate(360deg);
  }
}

/* Make the Turnstile iframe fill the container width */
.turnstile-container :deep(iframe) {
  width: 100% !important;
}
</style>
