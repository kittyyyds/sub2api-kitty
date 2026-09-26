<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'
import type { OpenAIRequestTimezoneSettings } from '@/utils/openAIRequestTimezone'

const props = defineProps<{ modelValue: OpenAIRequestTimezoneSettings }>()
const emit = defineEmits<{ 'update:modelValue': [value: OpenAIRequestTimezoneSettings] }>()
const { t } = useI18n()
const options = ref([{ value: props.modelValue.timezone, label: props.modelValue.timezone }])
const loading = ref(false)
const loadFailed = ref(false)
const loaded = ref(false)

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
        <span class="input-label">{{ t('admin.accounts.requestTimezone.target') }}</span>
        <Select :model-value="modelValue.timezone" :options="options" :loading="loading" searchable
          :aria-label="t('admin.accounts.requestTimezone.target')"
          @update:model-value="value => typeof value === 'string' && update({ timezone: value })" />
      </label>
      <p v-if="loadFailed" class="text-xs text-red-500" role="alert">{{ t('admin.accounts.requestTimezone.loadFailed') }}</p>
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
