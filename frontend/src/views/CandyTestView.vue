<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import CandyResult from '@/components/candy-test/CandyResult.vue'
import { candyQuestion, candyAnswers } from '@/components/candy-test/preset'
import { candyTestsAPI, candyStatusLabels, candyEffortLabels, type CandyCard, type CandyConfig, type CandyKey, type CandyRun } from '@/api/candyTests'

const props = withDefaults(defineProps<{ admin?: boolean }>(), { admin: false })
const cards = ref<CandyCard[]>([])
const keys = ref<CandyKey[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const autoRefresh = ref(true)
const filter = ref(0)
const refreshedAt = ref('')
const edit = ref<CandyConfig | null>(null)
const answerText = ref('')
const detail = ref<CandyRun | null>(null)
const history = ref<CandyCard | null>(null)
const runningID = ref(0)
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
let clockTimer: ReturnType<typeof setInterval> | undefined
let active: AbortController | undefined
let disposed = false
const visible = computed(() => cards.value.filter(c => !filter.value || c.id === filter.value))
const groups = computed(() => [...new Map(keys.value.map(k => [k.group_id, { id: k.group_id, name: k.group_name }])).values()])
const groupKeys = computed(() => keys.value.filter(k => k.group_id === edit.value?.group_id))
const stats = computed(() => {
  const latest = cards.value.map(c => c.history[0]?.status)
  return { total: cards.value.length, passed: latest.filter(s => s === 'passed').length,
    failed: latest.filter(s => s === 'failed').length, pending: latest.filter(s => s === 'queued' || s === 'running').length,
    other: latest.filter(s => s === 'error' || s === 'inconclusive').length }
})
const schedule = computed(() => {
  const enabled = cards.value.filter(c => c.enabled)
  if (!enabled.length) return '手动测试模式'
  const periods = [...new Set(enabled.map(c => c.interval_minutes))]
  return periods.length === 1 ? `每 ${periods[0]} 分钟自动测试` : `${enabled.length} 个分组按各自周期测试`
})
const time = (value: string | null) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '尚未测试'
const countdown = (card: CandyCard) => {
  if (!card.enabled) return '定时测试已关闭'
  if (['queued', 'running'].includes(card.history[0]?.status || '')) return '本轮等待 / 正在测试'
  if (!card.next_run_at) return '等待安排下次测试'
  const seconds = Math.max(0, Math.ceil((new Date(card.next_run_at).getTime() - now.value) / 1000))
  if (!seconds) return '已到测试时间，等待开始'
  return `距下次测试 ${String(Math.floor(seconds / 60)).padStart(2, '0')} 分 ${String(seconds % 60).padStart(2, '0')} 秒`
}
const message = (e: unknown) => (e as { message?: string })?.message || '请求失败，请稍后重试'

async function refresh() {
  if (loading.value || disposed) return
  loading.value = true
  const controller = new AbortController(); active = controller
  try {
    const payload = await candyTestsAPI.list(props.admin, controller.signal)
    if (disposed) return
    cards.value = payload.cards; refreshedAt.value = payload.updated_at; error.value = ''
  } catch (e) { if (!controller.signal.aborted) error.value = message(e) }
  finally { loading.value = false; if (active === controller) active = undefined }
}
function newConfig() {
  edit.value = { id: 0, group_id: 0, api_key_id: 0, label: '', model: 'gpt-5.6-sol', reasoning_effort: 'low', question: candyQuestion, expected_answers: [...candyAnswers], interval_minutes: 20, max_output_tokens: 8192, enabled: false, published: false }
  answerText.value = candyAnswers.join('\n'); notice.value = ''
}
function editConfig(card: CandyCard) {
  if (!card.config) return
  edit.value = { ...card.config, expected_answers: [...card.config.expected_answers] }
  answerText.value = card.config.expected_answers.join('\n'); notice.value = ''
}
function changeGroup() {
  if (!edit.value) return
  edit.value.api_key_id = 0
  edit.value.label = groups.value.find(g => g.id === edit.value?.group_id)?.name || ''
}
async function save() {
  if (!edit.value || saving.value) return
  saving.value = true; error.value = ''
  try {
    const value = await candyTestsAPI.save({ ...edit.value, expected_answers: answerText.value.split('\n').map(s => s.trim()).filter(Boolean) })
    notice.value = value.enabled ? '配置已保存，将在设定周期到期后测试。' : '配置已保存，定时测试关闭。可点击“测试一次”发起真实请求。'
    edit.value = null; await refresh()
  } catch (e) { error.value = message(e) }
  finally { saving.value = false }
}
async function run(card: CandyCard) {
  if (runningID.value) return
  runningID.value = card.id; error.value = ''; notice.value = ''
  try {
    await candyTestsAPI.run(card.id)
    notice.value = `${card.label} 已加入测试队列；会按测试 Key 的正常计费规则扣费。`
    await refresh()
  } catch (e) { error.value = message(e) }
  finally { runningID.value = 0 }
}
async function showHistory(card: CandyCard) {
  try {
    const records = await candyTestsAPI.history(card.id, props.admin)
    if (!disposed) history.value = { ...card, history: records }
  } catch (e) { if (!disposed) error.value = message(e) }
}
onMounted(async () => {
  await refresh()
  if (disposed) return
  if (props.admin) { try { keys.value = await candyTestsAPI.keys() } catch (e) { error.value = message(e) } }
  if (disposed) return
  timer = setInterval(() => { if (autoRefresh.value && document.visibilityState !== 'hidden') void refresh() }, 15000)
  clockTimer = setInterval(() => { now.value = Date.now() }, 1000)
})
onUnmounted(() => { disposed = true; active?.abort(); if (timer) clearInterval(timer); if (clockTimer) clearInterval(clockTimer) })
</script>

<template>
  <AppLayout>
    <main class="candy-page">
      <header class="page-heading">
        <div><p class="eyebrow">KITRTY · GROUP REASONING TEST</p><h1><span class="candy-symbol" aria-hidden="true">✦</span> 糖果测试</h1><p class="subtitle">用一道题，看看分组的真实回答。原文、答案核对与历史记录，一起呈现。</p></div>
        <div class="schedule-box"><span class="schedule-dot" /> <div><strong>{{ schedule }}</strong><p>刷新结果不会重新出题</p></div></div>
      </header>

      <div class="stats-strip">
        <div><span>测试分组</span><strong>{{ stats.total }}</strong></div>
        <div><span>本轮通过</span><strong class="green">{{ stats.passed }}</strong></div>
        <div><span>本轮未通过</span><strong class="red">{{ stats.failed }}</strong></div>
        <div><span>等待 / 正在回答</span><strong>{{ stats.pending }}</strong></div>
        <div><span>失败 / 待核对</span><strong>{{ stats.other }}</strong></div>
      </div>

      <details class="question-guide">
        <summary>糖果推理题 · 标准答案：21 <span>查看题目与判定说明</span></summary>
        <p>按题目“形状可凭手感分辨”的条件，选择 9 个圆形、12 个五角星形，共 21 个，可保证出现苹果味与桃子味的异形配对。下方保留每轮模型原文，方便自行核对。</p>
        <pre>{{ candyQuestion }}</pre>
        <p>本站按预设答案核对。若把题意理解为完全随机摸取、不能选择形状，会得到 29；一次答案差异不能单独证明模型被降级。</p>
      </details>

      <div v-if="error" role="alert" class="alert error">{{ error }}</div>
      <div v-if="notice" role="status" class="alert">{{ notice }}</div>

      <section v-if="admin" class="admin-bar">
        <div><strong>后台测试配置</strong><p>绑定管理员创建的分组 API Key。真实请求按该 Key 扣费，客户只查看已发布的结果。</p></div>
        <button class="btn btn-primary" @click="newConfig">＋ 添加测试分组</button>
      </section>

      <section v-if="admin && edit" class="editor">
        <div class="editor-title"><h2>{{ edit.id ? '编辑测试配置' : '添加分组测试' }}</h2><button type="button" class="btn btn-secondary" @click="edit = null">收起</button></div>
        <p v-if="!keys.length" class="help">暂无可用测试 Key。请用管理员账号在“API 密钥”页面创建一个 Key，绑定 GPT 分组，然后刷新此页面。</p>
        <form @submit.prevent="save">
          <div class="form-grid">
            <label>测试分组<select v-model.number="edit.group_id" class="input" :disabled="!!edit.id" required @change="changeGroup"><option :value="0" disabled>选择 GPT 分组</option><option v-if="edit.id && !groups.some(g => g.id === edit?.group_id)" :value="edit.group_id">原分组 #{{ edit.group_id }}（当前不可用）</option><option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option></select></label>
            <label>分组 API Key<select v-model.number="edit.api_key_id" class="input" required><option :value="0" disabled>选择管理员的测试 Key</option><option v-if="edit.api_key_id && !groupKeys.some(k => k.id === edit?.api_key_id)" :value="edit.api_key_id">原 Key #{{ edit.api_key_id }}（当前不可用）</option><option v-for="k in groupKeys" :key="k.id" :value="k.id">{{ k.name }} · #{{ k.id }}</option></select></label>
            <label>前台展示名称<input v-model="edit.label" class="input" maxlength="80" required /></label>
            <label>请求模型<input v-model="edit.model" class="input" maxlength="160" required placeholder="gpt-5.6-sol" /></label>
            <label>思考强度<select v-model="edit.reasoning_effort" class="input"><option v-for="(label, value) in candyEffortLabels" :key="value" :value="value">{{ label }}</option></select></label>
            <label>每隔多少分钟测试<input v-model.number="edit.interval_minutes" class="input" type="number" min="5" max="1440" required /></label>
            <label>请求输出 Token 上限<input v-model.number="edit.max_output_tokens" class="input" type="number" min="256" max="16384" required /></label>
          </div>
          <label class="full-field">糖果题原题<textarea v-model="edit.question" class="input" rows="5" required maxlength="8000" placeholder="粘贴完整题目。标准答案不要写在这里。" /></label>
          <label class="full-field">可接受的最终答案（每行一个）<textarea v-model="answerText" class="input" rows="2" required placeholder="例如不同但等价的表述，分别填写一行" /></label>
          <p class="help">题目原文会发送给分组接口；标准答案仅在本站核对。模型需在末行写“最终答案：…”，按完整答案匹配；格式缺失、截断或超时不判为答错。</p>
          <div class="switch-row"><label><input v-model="edit.enabled" type="checkbox" /> 开启定时测试（产生实际用量）</label><label><input v-model="edit.published" type="checkbox" /> 在客户前台展示此分组</label></div>
          <p class="help">每轮每组发起 1 个请求，最多保存最近 10 次。极高思考仍可能消耗较多 Token；输出上限由上游是否支持决定，实际费用以该 Key 的使用记录为准。保存新配置会取消尚未开始的旧任务，已发出的请求继续记录。测试从服务器内部调用，不经过公网 CDN。</p>
          <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中…' : '保存配置' }}</button>
        </form>
      </section>

      <div class="toolbar">
        <label class="group-picker">查看分组<select v-model.number="filter" class="input"><option :value="0">全部分组</option><option v-for="card in cards" :key="card.id" :value="card.id">{{ card.label }}</option></select></label>
        <div class="refresh-controls"><label><input v-model="autoRefresh" type="checkbox" /> 自动刷新结果</label><button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ loading ? '读取中…' : '刷新结果' }}</button></div>
      </div>
      <div class="list-caption"><span>各分组独立展示 · 每次为该分组的一次真实调用</span><span v-if="refreshedAt">数据更新 {{ time(refreshedAt) }}</span></div>

      <div v-if="!cards.length" class="empty-state"><div class="empty-star">✦</div><h2>{{ loading ? '正在读取测试记录' : '暂时没有发布的分组测试' }}</h2><p>{{ admin ? '添加分组、题目和标准答案，保存后即可测试。' : '管理员发布测试后，这里会展示真实回答和历史记录。' }}</p></div>
      <div class="cards-grid">
        <article v-for="card in visible" :key="card.id" class="test-card">
          <div class="card-heading">
            <div class="card-title"><h2>{{ card.label }}</h2><p>{{ card.model }} · {{ candyEffortLabels[card.reasoning_effort] }}思考</p></div>
            <div class="next-run">
              <span class="next-run-label">下次测试</span>
              <time v-if="card.enabled && card.next_run_at" :datetime="card.next_run_at">{{ time(card.next_run_at) }}</time>
              <strong aria-live="off">{{ countdown(card) }}</strong>
            </div>
          </div>
          <CandyResult v-if="card.history[0]" :run="card.history[0]" />
          <div v-else class="no-result">本分组尚未测试，等待第一份真实回答。</div>
          <div class="card-actions"><button v-if="card.history[0]" class="btn btn-secondary" @click="detail = card.history[0]">查看完整回答</button><button class="history-button" :disabled="!card.history_count" @click="showHistory(card)">历史 {{ card.history_count }} 次 ↗</button><template v-if="admin"><button class="btn btn-secondary" @click="editConfig(card)">编辑配置</button><button class="btn btn-primary" :disabled="!!runningID || ['queued','running'].includes(card.history[0]?.status || '')" @click="run(card)">测试一次（计费）</button></template></div>
          <p v-if="admin" class="publication-state">{{ card.config?.published ? '前台可见' : '仅后台可见' }}</p>
        </article>
      </div>
      <p class="page-footnote">通过表示这次回答与预设答案一致。单道题的结果不能证明模型真实身份或判断账号是否被降级；请求失败与答错分开记录。</p>
      <BaseDialog :show="!!detail" title="本轮完整回答" width="wide" @close="detail = null"><CandyResult v-if="detail" :run="detail" expanded /></BaseDialog>
      <BaseDialog :show="!!history" :title="`${history?.label || ''} · 最近 10 次测试`" width="wide" @close="history = null"><div class="history-list"><button v-for="run in history?.history || []" :key="run.id" @click="detail = run; history = null"><span>{{ time(run.created_at) }}</span><span>{{ candyStatusLabels[run.status] }}</span><span>查看原文 →</span></button></div></BaseDialog>
    </main>
  </AppLayout>
</template>

<style scoped>
.candy-page { --candy-text:#243546;--candy-muted:#748391;--candy-border:#e2e9eb;--candy-well:#f7fafb;--candy-tint:#f1fbf7; color:var(--candy-text); max-width:1600px; margin:0 auto; padding:12px 0 30px; }
.question-guide { border:1px solid var(--candy-border); background:var(--candy-well); border-radius:14px; padding:17px 20px; margin-bottom:22px; font-size:13px; line-height:1.85; }.question-guide summary { cursor:pointer; font-weight:650; }.question-guide summary span { color:var(--candy-muted); font-size:12px; font-weight:400; margin-left:12px; }.question-guide p { margin-top:12px; color:var(--candy-muted); }.question-guide pre { white-space:pre-wrap; overflow-wrap:anywhere; font:inherit; margin-top:14px; }
:global(.dark) .candy-page { --candy-text:#e2e8f0;--candy-muted:#98a8b9;--candy-border:#334155;--candy-well:#18232f;--candy-tint:#192f2c; }
.page-heading { display:flex; align-items:center; justify-content:space-between; gap:24px; padding:16px 0 28px; }.eyebrow { color:#369583; font-size:10px; letter-spacing:2px; font-weight:700; margin-bottom:9px; }h1 { font-size:30px; font-weight:700; letter-spacing:-.7px; display:flex; align-items:center; gap:10px; }.candy-symbol { color:#3aa38f; font-size:30px; }.subtitle { color:var(--candy-muted); font-size:13px; margin-top:10px; line-height:1.8; }
.schedule-box { display:flex; align-items:center; gap:12px; border:1px solid var(--candy-border); border-radius:14px; padding:15px 20px; background:var(--candy-well); white-space:nowrap; }.schedule-box strong { font-size:12px; }.schedule-box p { font-size:11px; color:var(--candy-muted); margin-top:4px; }.schedule-dot { width:7px; height:7px; border-radius:50%; background:#42b39b; box-shadow:0 0 0 5px #42b39b12; }
.stats-strip { display:grid; grid-template-columns:repeat(5,minmax(0,1fr)); background:var(--candy-well); border:1px solid var(--candy-border); border-radius:15px; padding:18px 4px; margin-bottom:26px; }.stats-strip>div { display:flex; align-items:center; justify-content:space-between; gap:8px; padding:0 22px; border-right:1px solid var(--candy-border); }.stats-strip>div:last-child { border:0; }.stats-strip span { font-size:12px; color:var(--candy-muted); }.stats-strip strong { font-size:28px; font-weight:650; font-variant-numeric:tabular-nums; }.green { color:#189e7b; }.red { color:#de706c; }
.toolbar,.refresh-controls,.group-picker { display:flex; align-items:center; gap:14px; }.toolbar { justify-content:space-between; margin:24px 0 14px; }.group-picker,.refresh-controls { font-size:12px; color:var(--candy-muted); }.group-picker select { width:190px; }.refresh-controls>label { display:flex; align-items:center; gap:7px; }input[type=checkbox] { accent-color:#239b85; width:15px; height:15px; }.list-caption { display:flex; justify-content:space-between; gap:16px; color:var(--candy-muted); font-size:11px; margin-bottom:12px; }
.cards-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:22px; }.test-card { min-width:0; border:1px solid var(--candy-border); background:var(--card-bg,#fff); border-radius:18px; padding:23px; box-shadow:0 5px 22px #263b4605; }.card-heading { display:flex; flex-wrap:wrap; align-items:flex-start; justify-content:space-between; gap:14px; margin-bottom:23px; }.card-title { flex:1 1 180px; min-width:0; }.card-heading h2 { font-size:20px; font-weight:650; overflow-wrap:anywhere; line-height:1.5; }.card-heading p { font-size:11px; color:var(--candy-muted); margin-top:5px; }.history-button { border:1px solid var(--candy-border); background:var(--candy-well); border-radius:9px; padding:8px 11px; color:var(--candy-muted); font-size:11px; white-space:nowrap; }.card-actions { display:flex; flex-wrap:wrap; gap:10px; margin-top:16px; }.card-actions button { font-size:12px; }.next-run { display:grid; gap:5px; margin-left:auto; max-width:100%; padding:10px 12px; border:1px solid var(--candy-border); border-radius:11px; background:var(--candy-tint); font-size:11px; text-align:right; font-variant-numeric:tabular-nums; color:var(--candy-text); }.next-run-label { color:var(--candy-muted); }.next-run strong { font-weight:600; }.publication-state { margin-top:14px; font-size:11px; color:var(--candy-muted); }.no-result { background:var(--candy-well); border-radius:12px; padding:65px 15px; text-align:center; font-size:13px; color:var(--candy-muted); }
.admin-bar,.editor { border:1px solid var(--candy-border); border-radius:14px; padding:20px; margin-bottom:18px; }.admin-bar { display:flex; justify-content:space-between; align-items:center; gap:15px; }.admin-bar strong { font-size:14px; }.admin-bar p,.help { font-size:12px; line-height:1.8; color:var(--candy-muted); margin:7px 0; }.editor-title { display:flex; justify-content:space-between; align-items:center; margin-bottom:18px; }.editor h2 { font-size:18px; font-weight:650; }.form-grid { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:16px; }.editor label { display:grid; gap:7px; font-size:12px; }.editor .full-field { margin-top:18px; }.switch-row { display:flex; gap:24px; flex-wrap:wrap; margin:20px 0 12px; }.switch-row label { display:flex; align-items:center; gap:8px; }.editor button[type=submit] { margin-top:12px; }.alert { padding:12px 16px; background:var(--candy-tint); border:1px solid var(--candy-border); border-radius:12px; margin-bottom:14px; font-size:13px; }.alert.error { color:#c85656; }
.empty-state { text-align:center; padding:70px 20px; border:1px dashed var(--candy-border); border-radius:18px; color:var(--candy-muted); }.empty-state h2 { font-size:17px; margin:10px 0; color:var(--candy-text); }.empty-state p { font-size:13px; }.empty-star { font-size:32px; color:#58a995; }.page-footnote { font-size:11px; color:var(--candy-muted); line-height:1.8; padding:24px 4px; }.history-list { display:grid; gap:8px; }.history-list button { display:flex; justify-content:space-between; gap:15px; border:1px solid var(--candy-border,#e2e8f0); border-radius:9px; padding:15px; font-size:12px; text-align:left; }
:global(.dark) .test-card { --card-bg:#1b2634; }:global(.dark) .candy-result { --candy-text:#e2e8f0;--candy-muted:#98a8b9;--candy-border:#334155;--candy-well:#18232f;--candy-tint:#192f2c; }
@media(max-width:1000px) { .stats-strip>div { padding:0 12px; }.stats-strip span { font-size:11px; }.form-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }.cards-grid { gap:15px; }.test-card { padding:17px; } }
@media(max-width:720px) { .page-heading { align-items:flex-start; flex-direction:column; gap:16px; }.schedule-box { padding:10px 14px; }.stats-strip { grid-template-columns:repeat(3,minmax(0,1fr)); row-gap:16px; }.stats-strip>div { border:0; }.stats-strip strong { font-size:23px; }.cards-grid,.form-grid { grid-template-columns:1fr; }.toolbar,.admin-bar { align-items:flex-start; flex-direction:column; }.list-caption { flex-direction:column; gap:6px; }.page-heading h1 { font-size:27px; }.history-list button { flex-wrap:wrap; } }
</style>
