<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { PricingFormEntry } from './types'
import { newVideoGroupPricing } from './videoGroupPricing'

const props = defineProps<{ modelValue: PricingFormEntry[] }>()
const emit = defineEmits<{ 'update:modelValue': [entries: PricingFormEntry[]] }>()
const { t } = useI18n()

function update(index: number, patch: Partial<PricingFormEntry>): void {
  emit('update:modelValue', props.modelValue.map((entry, i) => i === index ? { ...entry, ...patch, intervals: [] } : entry))
}
</script>

<template>
  <section class="mt-4 space-y-3 border-t border-gray-200 pt-4 dark:border-dark-400" data-testid="video-group-pricing">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('videoModule.pricingTitle') }}</h4>
      <button type="button" class="btn btn-secondary" data-testid="video-pricing-add" @click="emit('update:modelValue', [...modelValue, newVideoGroupPricing()])">{{ t('videoModule.addModel') }}</button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('videoModule.pricingHint') }}</p>
    <div v-for="(entry, index) in modelValue" :key="index" class="grid grid-cols-1 items-end gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-500 sm:grid-cols-[minmax(0,2fr)_minmax(6rem,1fr)_minmax(6rem,1fr)_auto]" data-testid="video-pricing-row">
      <label class="min-w-0 text-xs text-gray-600 dark:text-gray-300">{{ t('videoModule.model') }}
        <input :value="entry.models.join(', ')" class="input mt-1" required :placeholder="t('videoModule.modelPlaceholder')" data-testid="video-pricing-model" @input="update(index, { models: [($event.target as HTMLInputElement).value] })">
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('videoModule.billing') }}
        <select :value="entry.billing_mode" class="input mt-1" data-testid="video-pricing-billing" @change="update(index, { billing_mode: ($event.target as HTMLSelectElement).value as 'video' | 'per_request' })">
          <option value="per_request">{{ t('videoModule.perVideo') }}</option>
          <option value="video">{{ t('videoModule.perSecond') }}</option>
        </select>
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">{{ t('videoModule.price') }}
        <input :value="entry.per_request_price" type="number" min="0" step="any" required class="input mt-1" data-testid="video-pricing-price" @input="update(index, { per_request_price: ($event.target as HTMLInputElement).value })">
      </label>
      <button type="button" class="btn btn-secondary" :aria-label="t('videoModule.removeModel')" @click="emit('update:modelValue', modelValue.filter((_, i) => i !== index))">{{ t('common.delete') }}</button>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('videoModule.priceHint') }}</p>
  </section>
</template>
