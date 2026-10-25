<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import type { ModelPlazaResponse } from '@/api/modelPlaza'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import type { MediaCatalogPolicy } from './mediaCatalogPolicy'
import { mediaEntries, mediaMatches, mediaMoney, mediaPriceSummary, estimateMediaPrice, type MediaKind, type MediaPriceEntry } from './mediaPricing'

const props = defineProps<{ response: ModelPlazaResponse | null; loading?: boolean; error?: boolean;
  receivedAt?: string; preview?: boolean; personal?: boolean; initialKind?: MediaKind; initialQuery?: string; catalogPolicy?: MediaCatalogPolicy; initialGroupId?: number; initialDetail?: string }>()
const emit = defineEmits<{ refresh: [] }>()
const { isDarkMode } = useDocumentDarkMode()
const kind = ref<MediaKind>(props.initialKind ?? 'image')
const query = ref(props.initialQuery ?? '')
const groupId = ref(props.initialGroupId ? String(props.initialGroupId) : '')
const billingUnit = ref('')
const selectedKey = ref('')
const dialog = ref<HTMLDialogElement | null>(null)
const tierKey = ref('')
const quantity = ref(1)
const seconds = ref(0)
const referenceSeconds = ref(0)
const notice = ref('')
let noticeTimer: ReturnType<typeof setTimeout> | undefined
const entries = computed(() => mediaEntries(props.response, props.catalogPolicy))
const kinds = computed(() => entries.value.filter(e => e.kind === kind.value))
const groups = computed(() => [...new Map(kinds.value.map(e => [e.group.id, e.group])).values()])
const visible = computed(() => kinds.value.filter(e => (!groupId.value || String(e.group.id) === groupId.value)
  && (!billingUnit.value || e.unit === billingUnit.value) && mediaMatches(e, query.value)))
const selected = computed(() => entries.value.find(e => e.key === selectedKey.value))
const durationOptions = computed(() => selected.value?.document ? selected.value.document.secondOptions : selected.value?.model.capabilities?.second_options)
const durationRange = computed(() => selected.value?.document ? selected.value.document.seconds : selected.value?.model.capabilities?.seconds)
const standardTiers = computed(() => selected.value?.tiers.filter(t => !t.period) ?? [])
const estimate = computed(() => selected.value ? estimateMediaPrice(selected.value, tierKey.value, quantity.value, seconds.value, referenceSeconds.value) : null)
const receivedLabel = computed(() => {
  const date = new Date(props.receivedAt ?? '')
  return Number.isNaN(date.getTime()) ? '尚未获取' : date.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })
})
watch(kind, () => { groupId.value = ''; billingUnit.value = '' })
watch(() => props.initialKind, value => { if (value) kind.value = value })
watch(() => props.initialQuery, value => { query.value = value ?? '' })
watch(() => props.initialGroupId, value => { groupId.value = value ? String(value) : '' })
let openedDetail = ''
async function openRequestedDetail() {
  const id = props.initialDetail
  const group = props.initialGroupId
  if (!id || !group) return
  const key = JSON.stringify([group, id])
  if (openedDetail === key) return
  const matches = entries.value.filter(entry => entry.id === id && entry.group.id === group)
  if (matches.length !== 1) return
  await nextTick()
  if (!dialog.value || props.initialDetail !== id || props.initialGroupId !== group) return
  openedDetail = key
  showDetails(matches[0]!)
}
watch([entries, () => props.initialDetail, () => props.initialGroupId], openRequestedDetail, { flush: 'post' })
onMounted(openRequestedDetail)
function showDetails(entry: MediaPriceEntry) {
  selectedKey.value = entry.key
  tierKey.value = entry.tiers.find(t => !t.period && t.price !== null)?.key ?? ''
  quantity.value = 1
  seconds.value = entry.document
    ? entry.document.secondOptions?.[0] ?? entry.document.seconds?.min ?? 0
    : entry.model.capabilities?.second_options?.[0] ?? entry.model.capabilities?.seconds?.min ?? 0
  referenceSeconds.value = 0
  dialog.value?.showModal()
}
async function copyId(id: string) {
  try { await navigator.clipboard.writeText(id); notice.value = '模型名称已复制，可粘贴到画布模型搜索框' }
  catch { notice.value = '复制失败，请选中模型名称手动复制' }
  clearTimeout(noticeTimer); noticeTimer = setTimeout(() => { notice.value = '' }, 3500)
}
onBeforeUnmount(() => clearTimeout(noticeTimer))
defineExpose({ showDetails })
</script>

<template>
  <section class="media-catalog" :class="{ 'is-dark': isDarkMode }">
    <div v-if="preview" class="preview-banner"><span class="preview-dot" /> 本地预览 · 使用网站公开价目快照 · 不提交生成、不扣费</div>
    <header class="catalog-heading">
      <div><div class="eyebrow">KITTY · MODEL LIBRARY</div><h1>找到适合你的创作模型<span>。</span></h1>
        <p>先看能力与价格，再开始创作。图片、视频分开找，费用明明白白。</p></div>
      <button class="refresh-button" type="button" :disabled="loading" @click="emit('refresh')"><span aria-hidden="true">↻</span> {{ loading ? '正在刷新' : '刷新价格' }}</button>
    </header>
    <div class="catalog-toolbar">
      <div class="media-tabs" role="group" aria-label="模型类型">
        <button type="button" :aria-pressed="kind === 'image'" @click="kind = 'image'">图片模型 <small>{{ entries.filter(e => e.kind === 'image').length }}</small></button>
        <button type="button" :aria-pressed="kind === 'video'" @click="kind = 'video'">视频模型 <small>{{ entries.filter(e => e.kind === 'video').length }}</small></button>
      </div>
      <label class="search-box"><span aria-hidden="true">⌕</span><input v-model="query" aria-label="搜索模型名称" placeholder="搜索或粘贴完整模型名称"><button v-if="query" type="button" aria-label="清空搜索" @click="query = ''">×</button></label>
    </div>
    <div class="filters">
      <label>分组 <select v-model="groupId" aria-label="筛选分组"><option value="">全部分组</option><option v-for="group in groups" :key="group.id" :value="String(group.id)">{{ group.name }}</option></select></label>
      <label>计费 <select v-model="billingUnit" aria-label="筛选计费方式"><option value="">全部计费方式</option><option v-for="unit in kind === 'image' ? ['张'] : ['条', '秒']" :key="unit" :value="unit">按{{ unit }}计费</option></select></label>
      <span class="result-count">找到 <strong>{{ visible.length }}</strong> 个模型条目</span>
      <span class="received-time">价目获取 {{ receivedLabel }} · 北京时间</span>
    </div>
    <div class="price-notice"><span aria-hidden="true">ⓘ</span><p><strong>这里展示站内售价，不是上游成本。</strong>{{ personal ? '已登录：目录按接口返回的分组及适用倍率展示。' : '当前为公开价格；登录后的授权和个人价格以账号实际配置为准。' }} 价格目录不代表已获得调用权限，提交前请确认报价。</p></div>
    <div v-if="catalogPolicy" class="source-notice"><strong>按指定价表选型</strong><span>仅展示价表列出且本站目录精确匹配的模型。说明以 {{ catalogPolicy.capturedOn }} 核对的价表为依据；金额仍取本站加价后售价，不重复加价。</span></div>
    <div v-if="error" role="alert" class="error-notice">刷新失败，当前不是最新核验结果。{{ response ? '保留上一次价目供参考，请刷新后再使用。' : '请重试。' }}</div>
    <div v-if="loading && !response" class="empty-state" role="status">正在读取模型和价格…</div>
    <div v-else-if="!visible.length" class="empty-state"><strong>没有找到匹配的模型</strong><p>试试完整模型名称，或切换分组、图片／视频分类。</p><button type="button" @click="query = ''; groupId = ''; billingUnit = ''">清除筛选</button></div>
    <div v-else class="model-grid">
      <article v-for="entry in visible" :key="entry.key" class="model-card" data-testid="media-model-card">
        <div class="model-card-top"><span class="model-symbol" :class="entry.kind" aria-hidden="true">{{ entry.kind === 'image' ? '▧' : '▷' }}</span><span class="group-chip" :title="entry.group.name">{{ entry.group.name }}</span><span class="status-dot" :class="entry.status" :title="entry.status === 'priced' ? '价格已公布，不代表调用授权' : '暂不可用或缺少有效价格'" /></div>
        <h2 :title="entry.id">{{ entry.id }}</h2>
        <p class="model-description">{{ entry.model.description || (entry.document ? '价表未提供补充说明；保留原始模型名，具体售价见价格表。' : '暂无详细说明；请查看已公布的规格与计费信息，不按名称猜测功能。') }}</p>
        <div class="capability-tags"><span v-for="tag in entry.tags.slice(0, 3)" :key="tag">{{ tag }}</span><span v-if="!entry.tags.length">{{ entry.kind === 'image' ? '图片模型' : '视频模型' }}</span><span v-if="entry.document">价表已收录</span><span v-else-if="entry.model.capabilities?.generic">规格以上游支持为准</span></div>
        <p v-if="entry.catalogWarnings?.length" class="card-warning">{{ entry.catalogWarnings[0] }}</p>
        <div class="card-price"><span>{{ entry.variableTimePrice ? '标准时段售价' : '站内售价' }} · USD</span><strong>{{ mediaPriceSummary(entry) }}</strong><small v-if="entry.document?.conditionalReferencePricing">含参考素材的条件报价需另行核对</small><small v-else-if="entry.referencePrice !== null">参考视频另计 {{ mediaMoney(entry.referencePrice) }}/秒</small><small v-else-if="entry.hasReferenceVideo">参考视频费用待确认</small><small v-else>{{ entry.tiers.filter(t => !t.period).length > 1 ? '可展开查看分规格价格' : '按所选模型计费单位计算' }}</small></div>
        <div class="card-actions"><button type="button" :aria-label="`复制模型名称 ${entry.id}`" @click="copyId(entry.id)">复制名称</button><button type="button" class="details-button" @click="showDetails(entry)">价格与说明 <span aria-hidden="true">↗</span></button></div>
      </article>
    </div>
    <footer class="catalog-footer"><span>价格获取时间不等于上游变价时间；本页不自动扫描上游新模型。</span><span>不同计费单位不可直接比较高低。</span></footer>
    <Teleport to="body">
    <p v-if="notice" class="media-catalog copy-notice" :class="{ 'is-dark': isDarkMode }" role="status">{{ notice }}</p>
    <dialog ref="dialog" class="media-catalog model-dialog" :class="{ 'is-dark': isDarkMode }" aria-labelledby="model-detail-title" @close="selectedKey = ''" @click="event => { if (event.target === dialog) dialog?.close() }">
      <div v-if="selected" class="dialog-body">
        <header class="dialog-header"><div><span class="eyebrow">模型详情 / {{ selected.kind === 'image' ? '图片' : '视频' }}</span><h2 id="model-detail-title">{{ selected.id }}</h2><p>{{ selected.group.name }}</p></div><button type="button" autofocus aria-label="关闭模型详情" @click="dialog?.close()">×</button></header>
        <p class="detail-description"><strong v-if="selected.document">价表说明：</strong>{{ selected.model.description || (selected.document ? '价表未提供补充说明；不根据模型名称推断能力。' : '暂无经过核对的详细介绍。支持能力以接口返回和生成页面实际可选参数为准。') }}</p>
        <div v-if="selected.document" class="document-rules"><p v-for="note in selected.document.pricingNotes" :key="note">{{ note }}</p><small>来源：{{ catalogPolicy?.title }} · {{ catalogPolicy?.capturedOn }} · 第 {{ selected.document.sourceRows.join('、') }} 行；原文能力描述并非本站实测承诺。</small></div>
        <p v-for="warning in selected.catalogWarnings" :key="warning" class="error-notice" role="alert">{{ warning }}</p>
        <p v-if="selected.pricingConflict" role="alert" class="error-notice">同一分组的该模型返回了不一致的价格，已停止试算；请联系管理员核实，不会自动选择最低价。</p>
        <div v-if="!selected.document" class="spec-facts">
          <span v-for="tag in selected.tags" :key="tag">{{ tag }}</span>
          <span v-if="selected.model.capabilities?.resolutions?.length">分辨率 {{ selected.model.capabilities.resolutions.join(' / ') }}</span>
          <span v-if="selected.model.capabilities?.aspect_ratios?.length">比例 {{ selected.model.capabilities.aspect_ratios.join(' / ') }}</span>
          <span v-if="selected.model.capabilities?.second_options?.length">时长 {{ selected.model.capabilities.second_options.join(' / ') }} 秒</span>
          <span v-else-if="selected.model.capabilities?.seconds">时长 {{ selected.model.capabilities.seconds.min }}–{{ selected.model.capabilities.seconds.max }} 秒</span>
          <span v-if="selected.model.capabilities?.reference_images">参考图片 {{ selected.model.capabilities.reference_images.min }}–{{ selected.model.capabilities.reference_images.max }} 张</span>
          <span v-if="selected.hasReferenceVideo">参考视频最多 {{ selected.model.capabilities?.reference_videos?.max }} 个<span v-if="selected.model.capabilities?.max_reference_seconds">，合计 ≤{{ selected.model.capabilities.max_reference_seconds }} 秒</span></span>
        </div>
        <h3>完整价格表 <small>USD · {{ selected.variableTimePrice ? '含分时规则' : '站内售价' }}</small></h3>
        <table class="tier-table"><thead><tr><th>计价档位</th><th>售价 / 单位</th></tr></thead><tbody><tr v-for="tier in selected.tiers" :key="tier.key"><td>{{ tier.name }}<small v-if="tier.period">{{ tier.period }}</small></td><td>{{ mediaMoney(tier.price) }}<span v-if="tier.price !== null"> / {{ selected.unit }}</span></td></tr><tr v-if="selected.hasReferenceVideo"><td>参考视频附加费用</td><td>{{ mediaMoney(selected.referencePrice) }}<span v-if="selected.referencePrice !== null"> / 秒</span></td></tr></tbody></table>
        <section v-if="!selected.estimateBlocked" class="estimate-panel" aria-label="费用预估"><h3>算一算这次多少钱 <small>仅试算，不提交任务</small></h3>
          <div class="estimate-controls"><label>计价档位<select v-model="tierKey" aria-label="预估计价档位"><option v-for="tier in standardTiers" :key="tier.key" :value="tier.key">{{ tier.name }}</option></select></label>
            <label v-if="selected.unit !== '秒'">{{ selected.kind === 'image' ? '计划生成张数' : '计划生成条数' }}<input v-model.number="quantity" type="number" min="1" max="100" step="1" aria-label="计划生成数量"></label>
            <label v-if="selected.kind === 'video' && durationOptions?.length">生成时长<select v-model.number="seconds" aria-label="生成时长"><option v-for="value in durationOptions" :key="value" :value="value">{{ value }} 秒</option></select></label>
            <label v-else-if="selected.unit === '秒'">生成时长（秒）<input v-model.number="seconds" type="number" :min="durationRange?.min ?? 1" :max="durationRange?.max" step="1" aria-label="生成时长"></label>
            <label v-if="selected.hasReferenceVideo && selected.unit === '秒'">参考视频总秒数<input v-model.number="referenceSeconds" type="number" min="0" :max="selected.model.capabilities?.max_reference_seconds" step="0.1" aria-label="参考视频秒数"></label>
          </div>
          <div v-if="estimate" class="estimate-result"><div><span>生成费用 {{ mediaMoney(estimate.generation) }}</span><span v-if="referenceSeconds > 0">＋参考视频 {{ mediaMoney(estimate.reference) }}</span></div><strong>预计 {{ mediaMoney(estimate.total) }}</strong></div>
          <p v-else class="estimate-warning">{{ selected.variableTimePrice ? '含分时／峰时规则，此处不猜测实时总价，请在生成前确认报价。' : '当前参数或价格不完整，无法可靠试算。请检查数量、时长和计价档位。' }}</p>
          <p class="estimate-disclaimer">仅按展示档位计算，不代表调用授权或最终报价。规格价格不等于该规格必定可生成；实际参数以生成入口为准。实际用量、参考素材和其他适用规则可能影响费用，提交前请核对。素材延期存储费用不包含在此试算中。</p>
        </section>
        <p v-else class="estimate-warning">该模型存在需单独核对的规格或条件报价，暂不提供简化试算，避免给出不可靠总价。</p>
        <footer class="dialog-footer"><span>原始模型 ID 保持不变</span><button type="button" class="details-button" @click="copyId(selected.id)">复制名称，去画布搜索</button></footer>
      </div>
    </dialog>
    </Teleport>
  </section>
</template>

<style scoped>
.source-notice{display:flex;flex-wrap:wrap;gap:5px 12px;padding:0 2px 18px;font-size:11px;color:var(--mc-muted)}.source-notice strong{color:var(--mc-ink)}.document-rules{background:var(--mc-soft);padding:12px 14px;border-radius:9px;font-size:12px;margin:0 0 18px}.document-rules p{margin:4px 0}.document-rules small{display:block;font-size:10px;color:var(--mc-muted);margin-top:8px}.card-warning{font-size:10px;color:#a2602a;line-height:1.7;margin:0 0 12px}

.media-catalog{--mc-bg:var(--kc-panel,#fff);--mc-soft:var(--kc-panel-muted,#f8f5ef);--mc-line:var(--kc-line,#e6e2da);--mc-ink:var(--kc-ink,#1c1c19);--mc-muted:var(--kc-muted,#65665f);--mc-accent:var(--kc-accent-strong,#9c3f00);--mc-tint:var(--kc-accent-soft,#fef8f4);color:var(--mc-ink);font-size:14px;line-height:1.6} .media-catalog.is-dark{--mc-bg:var(--kc-panel,#202b3b);--mc-soft:var(--kc-panel-muted,#182334);--mc-line:var(--kc-line,#344052);--mc-ink:var(--kc-ink,#f7faf8);--mc-muted:var(--kc-muted,#bec9c4);--mc-accent:var(--kc-accent-strong,#ffd0bf);--mc-tint:var(--kc-accent-soft,#3e1600)}
button,input,select{font:inherit}button{cursor:pointer;transition:background .15s,color .15s}button:disabled{cursor:wait;opacity:.6}button:focus-visible,input:focus-visible,select:focus-visible{outline:2px solid var(--mc-accent);outline-offset:3px} .preview-banner{display:flex;align-items:center;gap:8px;padding:10px 14px;border:1px solid var(--mc-line);background:var(--mc-soft);color:var(--mc-muted);border-radius:10px;font-size:12px;margin-bottom:26px}.preview-dot,.status-dot{width:7px;height:7px;border-radius:50%;background:var(--mc-accent);flex-shrink:0}
.catalog-heading{display:flex;justify-content:space-between;align-items:center;gap:20px;margin:12px 0 32px}.eyebrow{font-size:10px;letter-spacing:2px;font-weight:700;color:var(--mc-accent)}h1{font-size:clamp(24px,3vw,36px);line-height:1.35;font-weight:650;letter-spacing:-1px;margin:10px 0}h1 span{color:var(--mc-accent)}.catalog-heading p{color:var(--mc-muted);font-size:13px;margin:8px 0 0}.refresh-button{white-space:nowrap;display:flex;align-items:center;gap:8px;border:1px solid var(--mc-line);border-radius:9px;background:var(--mc-bg);padding:9px 15px;font-size:12px}.refresh-button span{font-size:20px}
.catalog-toolbar{display:flex;justify-content:space-between;gap:20px;align-items:center;border-bottom:1px solid var(--mc-line);padding-bottom:20px}.media-tabs{display:flex;background:var(--mc-soft);border:1px solid var(--mc-line);padding:4px;border-radius:11px;gap:4px}.media-tabs button{padding:9px 16px;border-radius:8px;color:var(--mc-muted);white-space:nowrap}.media-tabs button[aria-pressed=true]{background:var(--mc-bg);color:var(--mc-accent);box-shadow:0 1px 4px #00000008}.media-tabs small{margin-left:8px;opacity:.7;font-size:11px}.search-box{display:flex;gap:10px;align-items:center;background:var(--mc-bg);border:1px solid var(--mc-line);border-radius:10px;width:min(390px,50%);padding:9px 13px}.search-box>span{font-size:22px;line-height:1;color:var(--mc-muted)}.search-box input{width:100%;min-width:0;background:transparent;outline:none;font-size:12px}.search-box button{color:var(--mc-muted)}
.filters{display:flex;gap:18px;align-items:center;flex-wrap:wrap;margin:18px 0 14px;color:var(--mc-muted);font-size:11px}.filters label{display:flex;gap:8px;align-items:center}.filters select{max-width:215px;background:var(--mc-bg);border:1px solid var(--mc-line);border-radius:7px;padding:5px 7px;color:var(--mc-ink);font-size:11px}.result-count strong{color:var(--mc-ink)}.received-time{margin-left:auto;font-size:10px}.price-notice{display:flex;align-items:flex-start;gap:10px;margin-bottom:22px;padding:12px 14px;background:var(--mc-tint);border-radius:9px;color:var(--mc-muted);font-size:11px}.price-notice>span{font-size:16px;color:var(--mc-accent)}.price-notice p{margin:0}.price-notice strong{color:var(--mc-accent);font-weight:600}
.model-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px}.model-card{display:flex;flex-direction:column;border:1px solid var(--mc-line);border-radius:13px;background:var(--mc-bg);padding:21px;min-width:0;transition:border-color .2s,transform .2s}.model-card:hover{border-color:var(--mc-accent);transform:translateY(-2px)}.model-card-top{display:flex;align-items:center;gap:10px;margin-bottom:17px}.model-symbol{width:36px;height:36px;display:grid;place-items:center;border-radius:10px;background:var(--mc-tint);color:var(--mc-accent);font-size:25px}.model-symbol.video{background:var(--mc-soft)}.group-chip{max-width:calc(100% - 64px);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--mc-muted);font-size:10px}.status-dot{margin-left:auto;width:6px;height:6px}.status-dot.pending,.status-dot.unsupported{background:#bf9350}h2{font-size:15px;font-weight:650;line-height:1.5;overflow-wrap:anywhere;margin:0 0 9px}.model-description{font-size:11px;color:var(--mc-muted);line-height:1.8;margin:0 0 14px;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;min-height:40px}.capability-tags{display:flex;gap:5px;flex-wrap:wrap;min-height:24px;margin-bottom:18px}.capability-tags span,.spec-facts>span{background:var(--mc-soft);border-radius:5px;padding:3px 7px;font-size:10px;color:var(--mc-muted)}.card-price{margin-top:auto;padding:16px 0 18px;border-top:1px solid var(--mc-line);display:flex;flex-direction:column;gap:5px}.card-price>span{font-size:10px;color:var(--mc-muted)}.card-price strong{font-size:23px;font-weight:600;letter-spacing:-.6px;font-variant-numeric:tabular-nums;color:var(--mc-accent)}.card-price small{font-size:10px;color:var(--mc-muted)}.card-actions{display:flex;align-items:center;justify-content:space-between;gap:10px;font-size:11px}.card-actions>button:first-child{color:var(--mc-muted);padding:7px 0}.details-button{border-radius:7px;padding:8px 11px;background:var(--kc-action,#1c211f);color:var(--kc-on-action,#f4f5f2);font-size:11px;font-weight:600}.details-button:hover{background:var(--kc-action-hover,#354139)}.details-button span{margin-left:10px}.catalog-footer{display:flex;justify-content:space-between;flex-wrap:wrap;gap:6px;font-size:10px;color:var(--mc-muted);margin-top:24px}.empty-state{padding:65px 20px;text-align:center;color:var(--mc-muted);border:1px dashed var(--mc-line);border-radius:12px}.empty-state strong{color:var(--mc-ink);font-size:17px}.empty-state p{font-size:12px;margin:10px}.empty-state button{color:var(--mc-accent);text-decoration:underline}.error-notice{color:#ad5a2d;background:#fff3e9;border:1px solid #ecd2bd;border-radius:9px;padding:12px;margin-bottom:18px;font-size:12px}.copy-notice{position:fixed;bottom:26px;left:50%;transform:translateX(-50%);max-width:90vw;background:var(--mc-ink);color:var(--mc-bg);border-radius:9px;padding:12px 20px;box-shadow:0 5px 30px #0002;z-index:1400;font-size:12px}
.model-dialog{color:var(--mc-ink);background:var(--mc-bg);border:1px solid var(--mc-line);border-radius:17px;width:min(700px,calc(100vw - 28px));max-height:90vh;padding:0;box-shadow:0 20px 80px #0003}.model-dialog::backdrop{background:rgb(15 24 21 / 40%);backdrop-filter:blur(4px)}.dialog-body{padding:28px}.dialog-header{display:flex;justify-content:space-between;gap:12px}.dialog-header h2{font-size:20px;margin:10px 0 3px}.dialog-header p{color:var(--mc-muted);font-size:12px}.dialog-header>button{width:30px;height:30px;font-size:24px;color:var(--mc-muted);flex-shrink:0}.detail-description{color:var(--mc-muted);font-size:12px;line-height:1.9;margin:18px 0}.spec-facts{display:flex;gap:7px;flex-wrap:wrap;margin-bottom:24px}h3{font-size:13px;font-weight:650;margin:0 0 12px}h3 small{font-size:10px;font-weight:400;color:var(--mc-muted);margin-left:8px}.tier-table{border-collapse:collapse;width:100%;font-size:12px;margin-bottom:24px}.tier-table th,.tier-table td{padding:10px 12px;border-bottom:1px solid var(--mc-line);text-align:left}.tier-table th{color:var(--mc-muted);font-weight:400;background:var(--mc-soft)}.tier-table td:last-child{color:var(--mc-accent);font-variant-numeric:tabular-nums}.tier-table small{display:block;color:var(--mc-muted);font-size:10px}.estimate-panel{background:var(--mc-soft);padding:18px;border:1px solid var(--mc-line);border-radius:10px}.estimate-controls{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.estimate-controls label{display:flex;flex-direction:column;gap:5px;font-size:10px;color:var(--mc-muted)}.estimate-controls input,.estimate-controls select{width:100%;min-width:0;border:1px solid var(--mc-line);border-radius:6px;padding:7px 9px;background:var(--mc-bg);color:var(--mc-ink);font-size:12px}.estimate-result{display:flex;justify-content:space-between;align-items:center;gap:10px;margin:18px 0 10px}.estimate-result>div{display:flex;flex-direction:column;font-size:11px;color:var(--mc-muted)}.estimate-result strong{font-size:23px;color:var(--mc-accent);white-space:nowrap}.estimate-disclaimer{font-size:10px;color:var(--mc-muted);line-height:1.8;margin:10px 0 0}.estimate-warning{font-size:12px;color:var(--mc-muted);margin:14px 0}.dialog-footer{display:flex;gap:14px;align-items:center;justify-content:space-between;margin-top:20px;font-size:10px;color:var(--mc-muted)}
@media(min-width:1450px){.model-grid{grid-template-columns:repeat(4,minmax(0,1fr))}}@media(max-width:1000px){.model-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.received-time{width:100%;margin-left:0}}@media(max-width:600px){.catalog-heading{align-items:flex-start;gap:10px;margin-bottom:24px}.catalog-heading p{font-size:11px}.refresh-button{padding:7px 9px;font-size:10px}.catalog-toolbar{flex-direction:column;align-items:stretch;gap:12px}.search-box{width:100%}.media-tabs button{flex:1}.model-grid{grid-template-columns:minmax(0,1fr)}.filters{gap:10px}.filters select{max-width:155px}.model-card{padding:19px}.dialog-body{padding:20px}.estimate-result{align-items:flex-start;flex-direction:column}.dialog-footer{align-items:flex-start}.preview-banner{font-size:10px}.received-time{font-size:9px}}
</style>
