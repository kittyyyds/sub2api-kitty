<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { imageUpstreamAPI, type ImageAdminState, type ImagePricingGroup } from '@/api/admin/imageUpstream'

const state = ref<ImageAdminState>()
const busy = ref(false)
const message = ref('')
const error = ref('')
const search = ref('')
const drafts = ref<Record<string, number | string>>({})
const modelKey = (group: number, model: string) => `${group}:${model}`
const rows = computed(() => (state.value?.groups || []).flatMap(group => (group.pricing || [])
  .filter(p => p.image?.provider === 'suanli' && p.models.length === 1)
  .map(p => ({ group, model: p.models[0]!, config: p.image!, key: modelKey(group.id, p.models[0]!) })))
  .filter(row => `${row.group.name} ${row.model}`.toLowerCase().includes(search.value.toLowerCase())))
const candidates = computed(() => Object.values(state.value?.catalog.models || {})
  .filter(m => /image|banana|香蕉/i.test(m.model) && m.model.toLowerCase().includes(search.value.toLowerCase()))
  .sort((a, b) => a.model.localeCompare(b.model)))
const date = (value?: string) => value ? new Date(value).toLocaleString() : '尚未读取'
const prices = (group: ImagePricingGroup, model: string) => group.prices[model.trim().toLowerCase()] || {}
async function load() { state.value = await imageUpstreamAPI.get() }
async function perform(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; message.value = ''
  try { await action() } catch (e: unknown) { error.value = e instanceof Error ? e.message : '操作失败，请稍后重试' }
  finally { busy.value = false }
}
function save() { return perform(async () => {
  if (!state.value) return
  await imageUpstreamAPI.save(state.value.settings)
  message.value = '图片设置已保存，视频设置不受影响。自动同步在下个周期按新规则执行。'
}) }
function sync(apply: boolean) { return perform(async () => {
  await imageUpstreamAPI.sync(apply); await load()
  message.value = apply ? '图片模型与售价已按保存规则同步，已有手动价格保留。' : '上游价格已读取，没有修改客户售价。'
}) }
function savePrice(group: number, model: string) { return perform(async () => {
  const value = drafts.value[modelKey(group, model)]
  const price = value === '' || value === undefined ? null : Number(value)
  if (price !== null && (!Number.isFinite(price) || price < 0)) throw new Error('请输入非负售价')
  await imageUpstreamAPI.price(group, model, price); await load()
  message.value = price === null ? '该型号已恢复自动跟价。' : '该型号已保存手动最终售价，自动同步不会覆盖。'
}) }
onMounted(() => perform(load))
</script>

<template>
  <div class="space-y-5" data-testid="image-upstream-panel">
    <p v-if="error" role="alert" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950">{{ error }}</p>
    <p v-if="message" role="status" class="rounded-xl bg-emerald-50 p-4 text-sm text-emerald-700 dark:bg-emerald-950">{{ message }}</p>
    <form v-if="state" class="card space-y-5 p-5" @submit.prevent="save">
      <h2 class="font-semibold">图片自动跟价</h2>
      <div class="flex flex-wrap gap-6 text-sm">
        <label><input v-model="state.settings.auto_sync" type="checkbox"> 自动同步图片售价</label>
        <label><input v-model="state.settings.auto_import_account_models" type="checkbox"> 按图片账号模型自动补齐分组</label>
        <label><input v-model="state.settings.follow_increase" type="checkbox"> 上游涨价时跟涨</label>
        <label><input v-model="state.settings.follow_decrease" type="checkbox"> 上游降价时跟降</label>
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="text-sm">每次出图加价<input v-model.number="state.settings.markup" aria-label="图片每次加价" type="number" min="0" step="0.00000001" required class="input mt-2"></label>
        <label class="text-sm">同步间隔（秒）<input v-model.number="state.settings.interval_seconds" type="number" min="30" max="86400" required class="input mt-2"></label>
      </div>
      <p class="text-xs text-gray-500">最终售价＝上游单价＋图片加价，不叠加分组倍率。上游当前按次报价，适配器每次生成一张（n=1）。1K/2K/4K价格格不代表渠道必然支持所有分辨率。读价失败沿用最后有效售价。</p>
      <p class="text-xs text-gray-500">在账号管理添加算力 AI 的 OpenAI 兼容 API Key，配置全部图片型号，绑定已开启图片生成的 OpenAI 分组。这里只同步该账号明确配置的型号；原有手动售价优先。</p>
      <button class="btn btn-primary" :disabled="busy">保存图片设置</button>
    </form>
    <section class="card space-y-4 p-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div><h2 class="font-semibold">图片上游价格与分组售价</h2><p class="mt-1 text-xs text-gray-500">最近成功：{{ date(state?.catalog.last_success) }}</p></div>
        <div class="flex gap-2"><button class="btn btn-secondary" :disabled="busy" @click="sync(false)">只读取上游价格</button><button class="btn btn-primary" :disabled="busy" @click="sync(true)">按已保存规则同步</button></div>
      </div>
      <p v-if="state?.catalog.last_error" class="text-sm text-amber-600">{{ state.catalog.last_error }}</p>
      <input v-model="search" class="input" placeholder="搜索图片型号或分组">
      <div class="overflow-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">分组 / 型号</th><th class="p-2">上游单价</th><th class="p-2">当前最终售价</th><th class="p-2">定价</th><th class="p-2">手动最终售价</th></tr></thead><tbody>
        <tr v-for="row in rows" :key="row.key" class="border-t border-gray-100 dark:border-dark-500">
          <td class="max-w-sm break-words p-2"><p class="text-xs text-gray-500">{{ row.group.name }}</p>{{ row.model }}<p v-if="row.config.description" class="mt-1 text-xs text-gray-500">上游说明：{{ row.config.description }}</p></td>
          <td class="p-2">{{ state?.catalog.models[row.config.upstream_model]?.unit_price ?? row.config.cost }} / 次</td>
          <td class="p-2"><div v-for="(price, tier) in prices(row.group, row.model)" :key="tier">{{ tier }}：{{ price }}</div></td>
          <td class="p-2">{{ row.config.auto_price ? '自动跟价' : '手动优先' }}</td>
          <td class="min-w-48 p-2"><input v-model="drafts[row.key]" type="number" min="0" step="0.00000001" class="input" placeholder="留空恢复自动跟价"><button class="btn btn-secondary mt-2" :disabled="busy" @click="savePrice(row.group.id, row.model)">保存此型号</button></td>
        </tr>
      </tbody></table><p v-if="!rows.length" class="py-6 text-gray-500">尚无已同步图片型号。绑定图片账号后，开启“按图片账号模型自动补齐分组”并保存，再同步即可。</p></div>
      <p class="text-xs text-gray-500">此处填写手动价将统一该型号的价格档。需要分辨率单独售价时，可在分组的图片型号价格格中修改，自动跟价会保留你的修改。</p>
    </section>
    <details class="card p-5"><summary class="cursor-pointer font-semibold">上游生图报价参考（{{ candidates.length }} 项）</summary><p class="mt-3 text-xs text-gray-500">此目录用于查价，不代表已经在本站开放；实际开放范围以绑定账号为准。</p><div class="mt-3 max-h-96 overflow-auto"><table class="w-full text-left text-sm"><tbody><tr v-for="m in candidates" :key="m.model" class="border-t border-gray-100 dark:border-dark-500"><td class="p-2">{{ m.model }}</td><td class="p-2">{{ m.unit_price }} / 次</td><td class="p-2">{{ m.description || '上游未提供说明' }}</td></tr></tbody></table></div></details>
  </div>
</template>
