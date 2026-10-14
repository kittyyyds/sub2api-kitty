<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'
import type { OpenAIRequestTimezoneSettings } from '@/utils/openAIRequestTimezone'
import type { ProxyTimezoneResult } from '@/api/admin/accounts'

const props = defineProps<{ modelValue: OpenAIRequestTimezoneSettings; proxyId?: number | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: OpenAIRequestTimezoneSettings] }>()
const { t } = useI18n()
const options = ref([{ value: props.modelValue.timezone, label: props.modelValue.timezone }])
const loading = ref(false)
const loadFailed = ref(false)
const loaded = ref(false)
const detecting = ref(false)
const detection = ref<ProxyTimezoneResult | null>(null)
const detectFailed = ref(false)
let detectionSequence = 0

async function detectProxy(force = false) {
  const sequence = ++detectionSequence
  detection.value = null
  detectFailed.value = false
  detecting.value = false
  if (!props.modelValue.enabled || props.modelValue.mode !== 'proxy' || !props.proxyId) return
  detecting.value = true
  try {
    const result = await adminAPI.accounts.probeOpenAIProxyTimezone(props.proxyId, force)
    if (sequence !== detectionSequence) return
    detection.value = result
    detectFailed.value = !result.success
  } catch {
    if (sequence === detectionSequence) detectFailed.value = true
  } finally {
    if (sequence === detectionSequence) detecting.value = false
  }
}

watch(() => [props.modelValue.enabled, props.modelValue.mode, props.proxyId], () => { void detectProxy() }, { immediate: true })
onBeforeUnmount(() => { detectionSequence++ })

function update(patch: Partial<OpenAIRequestTimezoneSettings>) {
  emit('update:modelValue', { ...props.modelValue, ...patch })
}

watch(() => props.modelValue.timezone, value => {
  if (!loaded.value) options.value = [{ value, label: value }]
})

watch(() => props.modelValue.enabled, async enabled => {
  if (!enabled || loaded.value || loading.value) return
  loading.value = true
  loadFailed.value = false
  try {
    const result = await adminAPI.accounts.getOpenAIRequestTimezones()
    options.value = result.timezones.map(value => ({ value, label: value }))
    loaded.value = true
    // Never silently rewrite an existing selection while opening an account.
    if (!result.timezones.includes(props.modelValue.timezone)) loadFailed.value = true
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}, { immediate: true })
</script>

<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="flex items-center justify-between gap-3">
      <span class="input-label mb-0">{{ t('admin.accounts.requestTimezone.enabled') }}</span>
      <input type="checkbox" :checked="modelValue.enabled" data-testid="request-timezone-enabled"
        class="h-4 w-4 rounded border-gray-300 text-primary-600"
        @change="update({ enabled: ($event.target as HTMLInputElement).checked })" />
    </label>
    <p class="input-hint">{{ t('admin.accounts.requestTimezone.hint') }}</p>
    <div v-if="modelValue.enabled" class="mt-3 space-y-3">
      <label class="block">
        <span class="input-label">{{ t('admin.accounts.requestTimezone.mode') }}</span>
        <Select :model-value="modelValue.mode || 'manual'" data-testid="request-timezone-mode"
          :options="[{ value: 'manual', label: t('admin.accounts.requestTimezone.manual') }, { value: 'proxy', label: t('admin.accounts.requestTimezone.proxy') }]"
          :aria-label="t('admin.accounts.requestTimezone.mode')"
          @update:model-value="value => (value === 'manual' || value === 'proxy') && update({ mode: value })" />
      </label>
      <label v-if="modelValue.mode !== 'proxy'" class="block">
        <span class="input-label">{{ t('admin.accounts.requestTimezone.target') }}</span>
        <Select :model-value="modelValue.timezone" :options="options" :loading="loading" searchable data-testid="request-timezone-target"
          :aria-label="t('admin.accounts.requestTimezone.target')"
          @update:model-value="value => typeof value === 'string' && update({ timezone: value })" />
      </label>
      <p v-if="loadFailed && modelValue.mode !== 'proxy'" class="text-xs text-red-500" role="alert">{{ t('admin.accounts.requestTimezone.loadFailed') }}</p>
      <div v-if="modelValue.mode === 'proxy'" class="space-y-2">
        <p v-if="!proxyId" class="input-hint">{{ t('admin.accounts.requestTimezone.noProxy') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="!proxyId || detecting"
          data-testid="request-timezone-detect" @click="detectProxy(true)">
          {{ t(detecting ? 'admin.accounts.requestTimezone.detecting' : 'admin.accounts.requestTimezone.detect') }}
        </button>
        <div aria-live="polite" role="status">
          <template v-if="detection?.success">
            <p class="break-all text-sm text-emerald-600 dark:text-emerald-400" data-testid="request-timezone-result">
              {{ [detection.ip_address, detection.country, detection.city, detection.timezone].filter(Boolean).join(' · ') }}
            </p>
            <p class="input-hint">{{ t('admin.accounts.requestTimezone.cacheHint') }}: {{ detection.checked_at }}</p>
          </template>
          <p v-else-if="detectFailed" class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.accounts.requestTimezone.detectFailed') }}</p>
        </div>
        <p class="input-hint">{{ t('admin.accounts.requestTimezone.proxyHint') }}</p>
      </div>
      <label class="flex items-center gap-2">
        <input type="checkbox" :checked="modelValue.syncSearch" data-testid="request-timezone-search"
          class="h-4 w-4 rounded border-gray-300 text-primary-600"
          @change="update({ syncSearch: ($event.target as HTMLInputElement).checked })" />
        <span class="text-sm">{{ t('admin.accounts.requestTimezone.syncSearch') }}</span>
      </label>
      <p class="input-hint">{{ t('admin.accounts.requestTimezone.searchHint') }}</p>
    </div>
  </div>
</template>
