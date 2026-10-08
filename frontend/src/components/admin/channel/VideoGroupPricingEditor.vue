<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { PricingFormEntry } from './types'
import { newVideoGroupPricing } from './videoGroupPricing'
import type { VideoModelConfig } from '@/api/admin/videoUpstream'

const props = defineProps<{ modelValue: PricingFormEntry[] }>()
const emit = defineEmits<{ 'update:modelValue': [entries: PricingFormEntry[]] }>()
const { t } = useI18n()

function update(index: number, patch: Partial<PricingFormEntry>): void {
  emit('update:modelValue', props.modelValue.map((entry, i) => i === index ? { ...entry, ...patch, intervals: [] } : entry))
}
function updateVideo(index: number, patch: Partial<VideoModelConfig>) {
  const video = props.modelValue[index]?.video
  if (video) update(index, { video: { ...video, ...patch } })
}
function overrideMarkup(index: number, enabled: boolean) {
  updateVideo(index, { markup: enabled ? { output_per_second: 0, reference_per_second: 0, per_request: 0 } : undefined })
}
function followValue(event: Event): boolean | undefined {
  const value = (event.target as HTMLSelectElement).value
  return value === '' ? undefined : value === 'true'
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
      <div v-if="entry.video" class="space-y-3 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800 sm:col-span-4">
        <div class="flex flex-wrap gap-5">
          <label><input :checked="entry.video.auto_price" type="checkbox" @change="updateVideo(index, { auto_price: ($event.target as HTMLInputElement).checked })"> 自动跟上游价格</label>
          <label><input :checked="!!entry.video.markup" type="checkbox" @change="overrideMarkup(index, ($event.target as HTMLInputElement).checked)"> 本型号单独加价</label>
          <label>跟涨 <select class="input" :value="entry.video.follow_increase === undefined ? '' : String(entry.video.follow_increase)" @change="updateVideo(index, { follow_increase: followValue($event) })"><option value="">使用全局</option><option value="true">开启</option><option value="false">关闭</option></select></label>
          <label>跟降 <select class="input" :value="entry.video.follow_decrease === undefined ? '' : String(entry.video.follow_decrease)" @change="updateVideo(index, { follow_decrease: followValue($event) })"><option value="">使用全局</option><option value="true">开启</option><option value="false">关闭</option></select></label>
        </div>
        <div v-if="entry.video.markup" class="grid gap-3 sm:grid-cols-3">
          <label>生成每秒加价<input :value="entry.video.markup.output_per_second" class="input mt-1" type="number" min="0" step="any" @input="updateVideo(index, { markup: { ...entry.video!.markup!, output_per_second: Number(($event.target as HTMLInputElement).value) } })"></label>
          <label>参考每秒加价<input :value="entry.video.markup.reference_per_second" class="input mt-1" type="number" min="0" step="any" @input="updateVideo(index, { markup: { ...entry.video!.markup!, reference_per_second: Number(($event.target as HTMLInputElement).value) } })"></label>
          <label>每条加价<input :value="entry.video.markup.per_request" class="input mt-1" type="number" min="0" step="any" @input="updateVideo(index, { markup: { ...entry.video!.markup!, per_request: Number(($event.target as HTMLInputElement).value) } })"></label>
        </div>
        <div class="grid gap-3 sm:grid-cols-3">
          <label>当前参考秒价<input :value="entry.video.reference_per_second" :disabled="entry.video.auto_price" class="input mt-1" type="number" min="0" step="any" @input="updateVideo(index, { reference_per_second: Number(($event.target as HTMLInputElement).value) })"></label>
          <label v-for="(price, resolution) in entry.video.resolution_prices" :key="resolution">{{ resolution }} 当前生成秒价<input :value="price" :disabled="entry.video.auto_price" class="input mt-1" type="number" min="0" step="any" @input="updateVideo(index, { resolution_prices: { ...entry.video!.resolution_prices, [resolution]: Number(($event.target as HTMLInputElement).value) } })"></label>
        </div>
        <p>自动跟价按已保存设置在下一次同步时应用；已提交任务不变价。手动定价可关闭自动跟价后编辑。</p>
      </div>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('videoModule.priceHint') }}</p>
  </section>
</template>
