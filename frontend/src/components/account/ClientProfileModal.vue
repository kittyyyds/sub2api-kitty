<template>
  <BaseDialog :show="show" :title="bulk ? '批量设置 GPT 指纹' : 'GPT 指纹配置'" width="wide" :show-close-button="!saving" :close-on-escape="!saving" @close="close">
    <form v-if="account || bulk" id="client-profile-form" class="space-y-6" @submit.prevent="save">
      <div class="rounded-xl border border-primary-200 bg-primary-50 p-4 text-sm text-primary-900 dark:border-primary-700/40 dark:bg-primary-900/20 dark:text-primary-100">
        <p class="font-medium">{{ bulk ? `已选择 ${accountIds.length} 个账号，仅应用于 GPT 授权账号` : `${account?.name} · 已保存方案：${clientProfileLabel(account?.extra)} · ${readClientProfile(account?.extra).enabled ? '账号独立配置' : '沿用原有配置'}` }}</p>
        <p class="mt-1 text-xs leading-5">{{ bulk ? '保存后将用下面的完整身份及连接方案覆盖选中 GPT 授权账号的方案，其他平台会跳过。' : '下方是待保存配置，不代表已有连接已经切换。' }}身份用于后续请求，连接参数用于新连接；已有请求正常完成。</p>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
      <p v-if="saveSummary" role="status" class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800">{{ saveSummary }}</p>
      <section class="space-y-3">
        <div>
          <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">客户端身份</h4>
          <p class="input-hint">方案用于模型请求及相关检测；登录授权、令牌刷新仍沿用全局客户端身份。不会修改模型、思考强度或对话关联。</p>
        </div>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="form.enabled" data-testid="profile-enabled" :disabled="saving" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
          启用独立方案（现有账号默认关闭）
        </label>
        <p class="input-hint">启用后，此账号使用内置 GPT 转发，不经过外部 OAuth 传输插件。</p>
        <fieldset :disabled="!form.enabled || saving" class="grid gap-3 disabled:opacity-60 md:grid-cols-2">
          <label class="input-label">身份方案
            <select v-model="form.preset" data-testid="profile-preset" class="input mt-1"><option v-for="option in presetOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
          </label>
          <label class="input-label">语言（留空保留原请求设置）
            <input v-model="form.language" class="input mt-1" placeholder="例如 zh-CN" />
          </label>
          <label class="input-label">客户端版本（留空沿用全局）
            <input v-model="form.client_version" class="input mt-1" placeholder="例如 0.146.0" />
          </label>
        </fieldset>
        <p class="input-hint">{{ presetOptions.find(option => option.value === form.preset)?.description }}。这是客户端声明，实际服务器系统和出口不会随之改变。</p>
        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <label class="input-label" for="profile-convergence">原有指纹收敛设置</label>
          <select id="profile-convergence" v-model="fingerprintMode" class="input" :disabled="saving">
            <option value="unchanged">保持原设置{{ !bulk ? `（${currentFingerprintLabel}）` : '（各账号分别保留）' }}</option>
            <option value="off">关闭收敛</option><option value="device">仅设备标识收敛</option><option value="session">会话收敛</option><option value="full">完全收敛</option>
          </select>
          <p class="input-hint">默认保留原设置。会话／完全收敛会改变关联标识，与系统身份预设是独立功能。</p>
        </div>
      </section>

      <section class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div><h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">连接设置（高级）</h4><p class="input-hint">只对新建连接生效；请求节奏、每分钟请求数和账号并发保持现状。</p></div>
        <fieldset :disabled="!form.enabled || saving" class="grid gap-3 disabled:opacity-60 md:grid-cols-2">
          <label class="input-label">HTTP 版本
            <select v-model="form.http_version" data-testid="profile-http" class="input mt-1"><option value="inherit">沿用现有</option><option value="http1">HTTP/1.1</option><option value="http2">HTTP/2（由上游协商）</option></select>
          </label>
          <label class="input-label">TLS 模板
            <select v-model="form.tls_profile" data-testid="profile-tls" class="input mt-1"><option value="inherit">沿用现有</option><option value="chrome">Chrome</option><option value="firefox">Firefox</option><option value="safari">Safari</option></select>
          </label>
          <label class="input-label">连接复用
            <select v-model="form.connection_reuse" class="input mt-1"><option value="inherit">沿用现有</option><option value="reuse">复用连接</option><option value="fresh">HTTP 新请求／WS 新会话新建连接</option></select>
          </label>
          <label class="input-label">WebSocket 模式
            <select v-model="form.ws_mode" class="input mt-1"><option value="inherit">沿用现有</option><option value="off">关闭</option><option value="ctx_pool">上下文连接池</option><option value="passthrough">透传</option></select>
          </label>
        </fieldset>
        <p class="text-xs leading-5 text-amber-700 dark:text-amber-300">自定义 TLS 模板目前走 HTTP/1.1，不能同时选择 HTTP/2。WebSocket 升级握手单独使用 HTTP/1.1；TLS 模板不等于完整操作系统指纹，实际连接方式仍受上游与代理支持限制。</p>
        <details class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <summary class="cursor-pointer text-sm font-medium text-gray-800 dark:text-gray-200">高级设置：连接池与 WebSocket 重试</summary>
          <p class="input-hint my-2">全部填 0 时沿用现有值。连接池上限不修改账号业务并发，但设得过低可能造成等待；WebSocket 重试只控制失败后的重连，续聊保留原连接。</p>
          <fieldset :disabled="!form.enabled || saving" class="grid gap-3 disabled:opacity-60 md:grid-cols-2">
            <label v-for="field in advancedFields" :key="field.key" class="input-label">{{ field.label }}<input v-model.number="form[field.key]" type="number" min="0" :max="field.max" step="1" class="input mt-1" /><span class="input-hint">0 沿用现有，上限 {{ field.max }}{{ field.unit }}</span></label>
          </fieldset>
        </details>
      </section>

      <section class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-3"><div><h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">出口信息</h4><p class="input-hint">时区是出口所在地推断值，仅记录，不向上游发送。代理失败时不会改用直连冒充结果。</p></div><button v-if="!bulk" type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="egressLoading || saving" @click="probeEgress">{{ egressLoading ? '检测中…' : '重新检测' }}</button></div>
        <p v-if="bulk" class="text-sm text-gray-500">各账号可能使用不同出口，请在单个账号的指纹配置中检测。</p>
        <template v-else>
          <label v-if="proxyChoices.length" class="input-label">检测哪个已绑定代理<select v-model="selectedProxy" class="input mt-1" :disabled="egressLoading"><option v-for="proxy in proxyChoices" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option></select></label>
          <p v-else class="text-sm text-gray-600 dark:text-gray-300">当前：服务器直连出口</p>
          <p v-if="egressError" role="alert" class="text-sm text-red-600 dark:text-red-300">{{ egressError }}</p>
          <dl v-if="egress" class="grid grid-cols-2 gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-700 dark:bg-dark-800/60">
            <div class="min-w-0 break-words"><dt class="text-gray-500">实际出口 IP</dt><dd class="mt-1 font-mono">{{ egress.ip }}</dd></div>
            <div class="min-w-0 break-words"><dt class="text-gray-500">推断时区 · 未发送</dt><dd class="mt-1">{{ egress.timezone || '未知' }}</dd></div>
            <div class="min-w-0 break-words"><dt class="text-gray-500">地区</dt><dd class="mt-1">{{ [egress.country, egress.region, egress.city].filter(Boolean).join(' / ') || '未知' }}</dd></div>
            <div class="min-w-0 break-words"><dt class="text-gray-500">检测时间</dt><dd class="mt-1">{{ formatDateTime(egress.observed_at) }}</dd></div>
          </dl>
          <p v-else class="rounded-lg bg-gray-50 p-3 text-sm text-gray-500 dark:bg-dark-800">当前出口尚无检测结果；重新检测只查询出口信息，不调用模型。</p>
        </template>
      </section>

      <p class="input-hint">用于客户端身份对照测试，不承诺解除风控、提高额度或改变上游模型分配。</p>
    </form>
    <template #footer><div class="flex w-full flex-wrap items-center justify-between gap-3"><button type="button" class="btn btn-secondary" :disabled="saving" @click="reset">恢复默认配置</button><div class="flex gap-2"><button type="button" class="btn btn-secondary" :disabled="saving" @click="close">取消</button><button type="submit" form="client-profile-form" class="btn btn-primary" :disabled="saving || (!account && !bulk)">{{ saving ? '保存中…' : bulk ? '应用到选中 GPT 账号' : '保存配置' }}</button></div></div></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import type { Account, Proxy } from '@/types'
import type { ClientProfileEgress } from '@/api/admin/accounts'
import { clientProfileLabel, defaultClientProfile, isClientProfileAccount, presetOptions, readClientProfile, validateClientProfile } from './clientProfile'
import { formatDateTime } from '@/utils/format'

const props = withDefaults(defineProps<{ show: boolean; account: Account | null; accountIds?: number[]; proxies?: Proxy[] }>(), { accountIds: () => [], proxies: () => [] })
const emit = defineEmits<{ (e: 'close'): void; (e: 'updated', account: Account): void }>()
const bulk = computed(() => !props.account && props.accountIds.length > 0)
const form = reactive(defaultClientProfile())
const saving = ref(false)
const error = ref('')
const saveSummary = ref('')
const fingerprintMode = ref('unchanged')
const currentFingerprintLabel = computed(() => ({ off: '关闭', device: '仅设备', session: '会话', full: '完全' })[String(props.account?.extra?.codex_fingerprint_mode || 'off') as 'off'] || '原设置')
const egressLoading = ref(false)
const egressError = ref('')
const egress = ref<ClientProfileEgress | null>(null)
const selectedProxy = ref<number | undefined>()
const proxyChoices = computed(() => {
  const ids = [...(props.account?.proxy_pool || [])]
  if (ids.length === 0 && props.account?.proxy_id) ids.push(props.account.proxy_id)
  return [...new Set(ids)].map(id => ({ id, name: props.proxies.find(proxy => proxy.id === id)?.name || props.account?.proxy?.name || `代理 #${id}` }))
})
const advancedFields = [
  { key: 'max_conns_per_host', label: '每个上游主机连接上限', max: 256, unit: '个' },
  { key: 'max_idle_conns_per_host', label: '每个上游主机空闲连接上限', max: 256, unit: '个' },
  { key: 'idle_conn_timeout_seconds', label: '空闲连接保留时间（秒）', max: 600, unit: '秒' },
  { key: 'retry_backoff_initial_ms', label: 'WebSocket 首次重试等待（毫秒）', max: 30000, unit: '毫秒' },
  { key: 'retry_backoff_max_ms', label: 'WebSocket 最大重试等待（毫秒）', max: 30000, unit: '毫秒' },
  { key: 'retry_total_budget_ms', label: 'WebSocket 重试总时间预算（毫秒）', max: 120000, unit: '毫秒' }
] as const

function message(cause: unknown, fallback: string): string {
  const detail = cause as { response?: { data?: { message?: string; error?: string } }; message?: string }
  return detail?.response?.data?.message || detail?.response?.data?.error || detail?.message || fallback
}
let loadSequence = 0
watch(() => [props.show, props.account?.id] as const, async ([show]) => {
  const sequence = ++loadSequence
  if (!show) return
  Object.assign(form, readClientProfile(props.account?.extra))
  fingerprintMode.value = 'unchanged'
  error.value = ''; saveSummary.value = ''; egressError.value = ''; egress.value = null
  selectedProxy.value = proxyChoices.value[0]?.id
  if (props.account) {
    try {
      const snapshot = await adminAPI.accounts.getClientProfileEgress(props.account.id)
      if (sequence === loadSequence && (!snapshot || snapshot.proxy_id === (selectedProxy.value ?? null))) egress.value = snapshot
    } catch (cause) { if (sequence === loadSequence) egressError.value = message(cause, '读取出口记录失败，可重新检测。') }
  }
}, { immediate: true })
watch(selectedProxy, () => { egress.value = null; egressError.value = '' })
function close() { if (!saving.value) emit('close') }
function reset() { Object.assign(form, defaultClientProfile()); fingerprintMode.value = 'unchanged'; error.value = '' }
async function save() {
  if (saving.value) return
  const validation = validateClientProfile(form)
  if (validation) { error.value = validation; return }
  error.value = ''; saveSummary.value = ''; saving.value = true
  let succeeded = 0; let skipped = 0
  const failures: string[] = []
  try {
    const ids = props.account ? [props.account.id] : [...props.accountIds]
    for (const id of ids) {
      try {
        if (bulk.value) {
          const account = await adminAPI.accounts.getById(id)
          if (!isClientProfileAccount(account)) { skipped++; continue }
        }
        const updated = await adminAPI.accounts.updateClientProfile(id, { ...form }, fingerprintMode.value === 'unchanged' ? undefined : fingerprintMode.value)
        emit('updated', updated); succeeded++
      } catch (cause) { failures.push(`#${id}：${message(cause, '保存失败')}`) }
    }
    if (failures.length) error.value = failures.join('；')
    if (bulk.value) saveSummary.value = `成功 ${succeeded} 个，跳过非 GPT 授权账号 ${skipped} 个，失败 ${failures.length} 个。`
    else if (!failures.length) emit('close')
  } finally { saving.value = false }
}
async function probeEgress() {
  if (!props.account || egressLoading.value) return
  const accountId = props.account.id; const proxyId = selectedProxy.value; const sequence = loadSequence
  egressLoading.value = true; egressError.value = ''; egress.value = null
  try {
    const snapshot = await adminAPI.accounts.probeClientProfileEgress(accountId, proxyId)
    if (sequence === loadSequence && props.account?.id === accountId && selectedProxy.value === proxyId) egress.value = snapshot
  } catch (cause) { if (sequence === loadSequence) egressError.value = message(cause, '出口检测失败') }
  finally { egressLoading.value = false }
}
</script>
