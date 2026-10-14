<script setup lang="ts">
import type { GrokMediaProvider } from './grokMediaProvider'
import { useI18n } from 'vue-i18n'

defineProps<{ modelValue: GrokMediaProvider; videoPlatform?: boolean }>()
const { t } = useI18n()
const emit = defineEmits<{
  'update:modelValue': [value: GrokMediaProvider]
  'use-suanli-url': []
}>()
</script>

<template>
  <div data-testid="grok-media-provider-field">
    <label class="input-label" for="grok-media-provider">{{ videoPlatform ? t('videoModule.provider') : '媒体上游' }}</label>
    <select
      id="grok-media-provider"
      :value="modelValue"
      class="input"
      data-testid="grok-media-provider"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value as GrokMediaProvider)"
    >
      <option value="">{{ videoPlatform ? t('videoModule.standardProvider') : 'Grok 标准接口' }}</option>
      <option value="suanli_special">{{ t('videoModule.suanliProvider') }}</option>
    </select>
    <p v-if="modelValue === 'suanli_special'" class="input-hint">
      {{ t('videoModule.providerHint') }}
      <button type="button" class="text-primary-600 hover:underline" @click="emit('use-suanli-url')">{{ t('videoModule.useSuanliUrl') }}</button>
    </p>
  </div>
</template>
