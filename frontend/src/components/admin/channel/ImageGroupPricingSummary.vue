<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { imageUpstreamAPI, type ImageAdminState } from '@/api/admin/imageUpstream'
import type { PricingFormEntry } from './types'

const props = defineProps<{
  pricing: PricingFormEntry[]
  prices: Record<string, Record<string, number | string | null | undefined>>
}>()
const state = ref<ImageAdminState>()
const failed = ref(false)
const rows = computed(() => props.pricing.filter(p => p.image?.provider === 'suanli' && p.models.length === 1).map(p => {
  const config = p.image!
  const tiers = props.prices[p.models[0]!.trim().toLowerCase()] ?? {}
  const populated = Object.entries(tiers).filter(([, value]) => value != null && value !== '')
  const empty = populated.length === 0
  const edited = !empty && (populated.length !== Object.keys(config.applied_prices).length
    || populated.some(([tier, value]) => Number(value) !== config.applied_prices[tier]))
  return { model: p.models[0]!, config, tiers: Object.fromEntries(populated), empty,
    automatic: empty || (config.auto_price && !edited),
    upstream: state.value?.catalog.models[config.upstream_model]?.unit_price ?? config.cost }
}))
onMounted(async () => {
  try { state.value = await imageUpstreamAPI.get() } catch { failed.value = true }
})
</script>

<template>
  <section class="mt-4 space-y-3 border-t border-gray-200 pt-4 dark:border-dark-400" data-testid="image-group-pricing">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h4 class="text-sm font-medium">图片模型上游价格与自动跟价</h4>
      <RouterLink to="/admin/video-upstream?section=image" target="_blank" rel="noopener" class="text-sm text-primary-600">设置图片加价 / 手动最终售价</RouterLink>
    </div>
    <p class="text-xs text-gray-500">型号跟随绑定账号，无需在分组重复填写。自动售价＝跟价规则采用的上游价＋图片加价；已有手动价格优先。加价、跟涨/跟跌及手动价在图片跟价页设置，清空手动价可恢复自动。视频设置独立。</p>
    <p class="text-xs text-gray-500">1K / 2K / 4K 为本站计费档位；同一按次型号可同价，实际可生成尺寸以上游该型号说明为准。</p>
    <p v-if="state" class="text-xs text-gray-500" data-testid="image-group-sync-state">自动跟价：{{ state.settings.auto_sync ? '开启' : '关闭' }} · 自动补齐：{{ state.settings.auto_import_account_models ? '开启' : '关闭' }} · 每次加价 {{ state.settings.markup }} · 间隔 {{ state.settings.interval_seconds }} 秒</p>
    <p v-if="failed" class="text-xs text-amber-600">暂未取得最新价格状态，以下显示分组已保存价格。</p>
    <div v-if="rows.length" class="max-h-80 overflow-auto">
      <table class="w-full text-left text-xs"><thead><tr><th class="p-2">账号型号</th><th class="p-2">上游价 / 次</th><th class="p-2">当前最终售价</th><th class="p-2">定价方式</th></tr></thead><tbody>
        <tr v-for="row in rows" :key="row.model" class="border-t border-gray-100 dark:border-dark-500">
          <td class="max-w-sm break-words p-2">{{ row.model }}<p v-if="row.config.description" class="mt-1 text-gray-500">{{ row.config.description }}</p></td>
          <td class="p-2">{{ row.upstream }}</td>
          <td class="p-2"><p v-if="row.empty">等待自动补齐</p><p v-for="(price, tier) in row.tiers" :key="tier">{{ tier }}：{{ price }}</p></td>
          <td class="p-2">{{ row.automatic ? (state?.settings.auto_sync === false ? '自动跟价已暂停' : '自动跟价') : '手动优先' }}</td>
        </tr>
      </tbody></table>
    </div>
    <p v-else class="text-xs text-gray-500">尚无已同步图片型号。保存分组并绑定图片账号后，开启图片自动补齐和跟价即可在下个同步周期显示；首次未探测到有效价格的型号不会免费接单。</p>
  </section>
</template>
