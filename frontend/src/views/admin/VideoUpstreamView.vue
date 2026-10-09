<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ImageUpstreamPanel from './ImageUpstreamPanel.vue'
import { videoUpstreamAPI, type VideoRuntimeSettings, type VideoCatalog } from '@/api/admin/videoUpstream'
import { list as listGroups } from '@/api/admin/groups'

const settings = ref<VideoRuntimeSettings>()
const section = ref<'video' | 'image'>('video')
const catalog = ref<VideoCatalog>()
const groups = ref<{ id: number; name: string }[]>([])
const group = ref<number>()
const search = ref('')
const selected = ref<string[]>([])
const busy = ref(false)
const message = ref('')
const error = ref('')
const models = computed(() => Object.values(catalog.value?.models || {}).filter(m => m.model.toLowerCase().includes(search.value.toLowerCase())).sort((a, b) => a.model.localeCompare(b.model)))
const changes = computed(() => [...(catalog.value?.changes || [])].reverse())
const date = (value: string) => value ? new Date(value).toLocaleString() : '尚未同步'
async function perform(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; message.value = ''
  try { await action() } catch (e: unknown) { error.value = e instanceof Error ? e.message : '操作失败，请稍后重试' }
  finally { busy.value = false }
}
async function load() {
  const [data, list] = await Promise.all([videoUpstreamAPI.get(), listGroups(1, 100, { platform: 'video' })])
  settings.value = data.settings; catalog.value = data.catalog; groups.value = list.items
}
function save() { return perform(async () => { await videoUpstreamAPI.save(settings.value!); message.value = '配置已保存，仅影响后续任务；现有任务保持提交时价格。' }) }
function sync() { return perform(async () => { catalog.value = await videoUpstreamAPI.sync(); message.value = '目录已同步，已开启跟价的型号按保存的涨跌规则更新。' }) }
function importModels() { return perform(async () => {
  if (!group.value || !selected.value.length) throw new Error('请选择视频分组和型号')
  await videoUpstreamAPI.import(group.value, selected.value)
  selected.value = []; message.value = '型号价格已导入。请在上游账号的模型白名单中开放相同型号，再进行调用验收。'
}) }
onMounted(() => perform(load))
</script>

<template>
  <AppLayout>
    <div class="space-y-6">
      <div><h1 class="text-2xl font-semibold text-gray-900 dark:text-white">上游与自动跟价</h1><p class="mt-2 text-sm text-gray-500">视频与图片分别设置、分别保存；手动售价优先。</p></div>
      <div class="flex gap-3" role="tablist" aria-label="媒体类型"><button role="tab" :aria-selected="section === 'video'" class="btn" :class="section === 'video' ? 'btn-primary' : 'btn-secondary'" @click="section = 'video'">视频跟价</button><button role="tab" :aria-selected="section === 'image'" class="btn" :class="section === 'image' ? 'btn-primary' : 'btn-secondary'" @click="section = 'image'">图片跟价</button></div>
      <ImageUpstreamPanel v-show="section === 'image'" />
      <div v-show="section === 'video'" class="space-y-6">
      <p v-if="error" role="alert" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950">{{ error }}</p>
      <p v-if="message" role="status" class="rounded-xl bg-emerald-50 p-4 text-sm text-emerald-700 dark:bg-emerald-950">{{ message }}</p>
      <form v-if="settings" class="card space-y-5 p-5" @submit.prevent="save">
        <h2 class="font-semibold">自动跟价与加价</h2>
        <div class="flex flex-wrap gap-6 text-sm">
          <label><input v-model="settings.auto_sync" type="checkbox"> 自动同步价格</label>
          <label><input v-model="settings.auto_import_account_models" type="checkbox"> 按账号模型自动补齐分组</label>
          <label><input v-model="settings.follow_increase" type="checkbox"> 上游涨价时跟涨</label>
          <label><input v-model="settings.follow_decrease" type="checkbox"> 上游降价时跟降</label>
        </div>
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <label class="text-sm">生成每秒加价<input v-model.number="settings.markup.output_per_second" type="number" min="0" step="0.00000001" required class="input mt-2"></label>
          <label class="text-sm">参考每秒加价<input v-model.number="settings.markup.reference_per_second" type="number" min="0" step="0.00000001" required class="input mt-2"></label>
          <label class="text-sm">每条加价<input v-model.number="settings.markup.per_request" type="number" min="0" step="0.00000001" required class="input mt-2"></label>
          <label class="text-sm">同步间隔（秒）<input v-model.number="settings.interval_seconds" type="number" min="30" max="86400" required class="input mt-2"></label>
        </div>
        <p class="text-xs text-gray-500">金额与本站视频余额同一口径。按条、按秒分别适用，不重复叠加。参考秒加价只用于已确认参考视频另收费的型号。单模型覆盖请到分组价格编辑。读价失败时继续沿用最后有效价格。</p>
        <h2 class="font-semibold">临时参考素材与并发</h2>
        <p class="text-xs text-gray-500">自动补齐以绑定账号中配置的全部确切模型为准，遵守账号的分组模型限制；在下个同步周期内生效。已有手动售价优先，新型号沿用加价设置；首次缺价的型号不会免费接单。</p>
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <label class="text-sm">保留时间（小时）<input v-model.number="settings.reference_retention_hours" class="input mt-2" type="number" min="1" max="168" required></label>
          <label class="text-sm">每个用户视频任务并发<input v-model.number="settings.max_concurrent_tasks" class="input mt-2" type="number" min="1" max="100" required></label>
          <label class="text-sm">每用户素材容量（MB）<input v-model.number="settings.reference_user_mb" class="input mt-2" type="number" min="50" max="10240" required></label>
          <label class="text-sm">本站素材总容量（MB）<input v-model.number="settings.reference_total_mb" class="input mt-2" type="number" :min="settings.reference_user_mb" max="102400" required></label>
        </div>
        <p class="text-xs text-gray-500">未使用素材按上传时间到期；已使用素材在最后关联任务结束后继续保留上述时长。在途任务不会被清理。生成成片不占用这些配额。</p>
        <button class="btn btn-primary" :disabled="busy">{{ busy ? '处理中…' : '保存设置' }}</button>
      </form>
      <section class="card space-y-4 p-5">
        <div class="flex flex-wrap items-center justify-between gap-4"><div><h2 class="font-semibold">上游模型目录</h2><p class="mt-1 text-xs text-gray-500">最近成功：{{ date(catalog?.last_success || '') }} · 参数与价格可解析不代表已完成真实生成验收</p></div><button class="btn btn-secondary" :disabled="busy" @click="sync">立即同步并按保存规则调价</button></div>
        <p v-if="catalog?.last_error" class="text-sm text-amber-600">{{ catalog.last_error }}</p>
        <div class="flex flex-wrap gap-3"><input v-model="search" class="input flex-1" placeholder="搜索型号"><select v-model="group" class="input sm:w-64"><option :value="undefined">选择视频分组</option><option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option></select><button class="btn btn-primary" :disabled="busy || !selected.length || !group" @click="importModels">导入所选 {{ selected.length }} 项</button></div>
        <p v-if="!groups.length" class="text-sm text-gray-500">请先在分组管理中建立视频分组，并配置算力上游账号。</p>
        <div class="max-h-[32rem] overflow-auto"><table class="w-full text-left text-sm"><thead class="sticky top-0 bg-white dark:bg-dark-800"><tr><th class="p-2">选择</th><th>型号</th><th>上游输出价</th><th>上游参考价</th><th>适配情况</th></tr></thead><tbody><tr v-for="m in models" :key="m.model" class="border-t border-gray-100 dark:border-dark-500"><td class="p-2"><input v-model="selected" type="checkbox" :value="m.model" :disabled="!!m.unavailable_reason || !m.spec?.supported"></td><td class="max-w-sm break-words p-2">{{ m.model }}</td><td class="whitespace-nowrap p-2">{{ m.unit_price }} / {{ m.billing_mode === 'per_request' ? '条' : '秒' }}</td><td class="p-2">{{ m.reference_per_second }} / 秒</td><td class="max-w-xs p-2">{{ m.unavailable_reason || m.spec?.reason || (m.spec?.supported ? '协议参数可用，开放前请实测' : '待核实') }}</td></tr></tbody></table><p v-if="!models.length" class="py-8 text-center text-gray-500">暂无目录，请同步价格。</p></div>
      </section>
      <section class="card space-y-4 p-5"><h2 class="font-semibold">最近调价记录</h2><div class="max-h-80 overflow-auto"><table class="w-full text-left text-sm"><thead><tr><th>时间</th><th>分组</th><th>型号</th><th>输出单价</th><th>参考秒价</th></tr></thead><tbody><tr v-for="(c, i) in changes" :key="i" class="border-t border-gray-100 dark:border-dark-500"><td class="py-2">{{ date(c.at) }}</td><td>{{ c.group_id }}</td><td>{{ c.model }}</td><td>{{ c.old }} → {{ c.new }}</td><td>{{ c.old_reference }} → {{ c.new_reference }}</td></tr></tbody></table><p v-if="!changes.length" class="py-6 text-gray-500">暂无自动调价记录</p></div></section>
      </div>
    </div>
  </AppLayout>
</template>
