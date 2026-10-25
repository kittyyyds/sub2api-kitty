<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import '../styles/console-theme.css'
import { useConsoleTheme } from '../composables/useConsoleTheme'
import type { ModelPlazaResponse } from '../api/modelPlaza'
import MediaPricingCatalog from '../components/modelPlaza/MediaPricingCatalog.vue'
import { suanliMediaCatalog } from '../components/modelPlaza/mediaCatalogPolicy'
import { mediaEntries, mediaMatches, mediaPriceSummary, type MediaPriceEntry } from '../components/modelPlaza/mediaPricing'
useConsoleTheme()
const data = ref<ModelPlazaResponse | null>(null)
const captured = ref('')
const loading = ref(false)
const error = ref(false)
const view = ref('catalog')
const dark = ref(false)
const catalog = ref<InstanceType<typeof MediaPricingCatalog>>()
const modelSearch = ref('')
const selectionKey = ref('')
const pickerKind = ref<'image' | 'video'>('video')
const entries = computed(() => mediaEntries(data.value, suanliMediaCatalog))
const options = computed(() => entries.value.filter(e => e.kind === pickerKind.value && mediaMatches(e, modelSearch.value)))
const selected = computed(() => entries.value.find(e => e.key === selectionKey.value))
function pick(entry: MediaPriceEntry) { selectionKey.value = entry.key }
function toggleDark() { dark.value = !dark.value; document.documentElement.classList.toggle('dark', dark.value) }
async function load() {
  if (loading.value) return
  loading.value = true; error.value = false
  try { const response = await fetch('/preview-data.json', { cache: 'no-store' }); if (!response.ok) throw new Error('snapshot unavailable');
    const snapshot = await response.json(); data.value = snapshot.data; captured.value = snapshot.captured_at }
  catch { error.value = true } finally { loading.value = false }
}
onMounted(load)
</script>
<template>
  <div class="preview-app">
    <nav class="preview-nav"><a href="/pricing-preview.html" class="preview-brand"><span>K</span>Kitty<span class="brand-sub">奇妙屋</span></a><div><button :aria-pressed="view === 'catalog'" @click="view = 'catalog'">模型与价格</button><button :aria-pressed="view === 'canvas'" @click="view = 'canvas'">画布选型预览</button></div><button class="theme-button" @click="toggleDark">{{ dark ? '浅色' : '深色' }}模式</button></nav>
    <main>
      <div v-if="view === 'canvas'" class="canvas-preview">
        <div class="canvas-intro"><span>无限画布 / 选型展示预览</span><h1>选模型的时候，价格就在手边。</h1><p>这里演示选择器与价格详情的联动，不加载、修改或执行你的线上画布节点。</p></div>
        <section class="picker-preview"><header><strong>视频 / 图片创作</strong><span>仅预览 · 不生成</span></header>
          <div class="picker-tabs"><button :aria-pressed="pickerKind === 'image'" @click="pickerKind = 'image'; modelSearch = ''; selectionKey = ''">图片</button><button :aria-pressed="pickerKind === 'video'" @click="pickerKind = 'video'; modelSearch = ''; selectionKey = ''">视频</button></div>
          <input v-model="modelSearch" aria-label="画布搜索模型" placeholder="搜索或粘贴模型名称">
          <div class="picker-list"><button v-for="entry in options" :key="entry.key" :aria-pressed="selected?.key === entry.key" @click="pick(entry)"><strong>{{ entry.id }}</strong><small>{{ entry.group.name }}</small><span>{{ mediaPriceSummary(entry) }}</span></button><p v-if="!options.length">未找到匹配模型</p></div>
          <div v-if="selected" class="picker-selected"><span>已选：{{ selected.id }}</span><strong>{{ mediaPriceSummary(selected) }}</strong><button @click="catalog?.showDetails(selected)">查看完整价格与说明 ↗</button></div>
          <p class="picker-footnote">生成按钮不会在预览中启用；价格目录不代表账号调用授权。</p>
        </section>
      </div>
      <div v-show="view === 'catalog'"><MediaPricingCatalog ref="catalog" :response="data" :loading="loading" :error="error" :received-at="captured" :catalog-policy="suanliMediaCatalog" preview @refresh="load" /></div>
    </main>
  </div>
</template>
<style>
body{margin:0}button{border:0;background:none}*{box-sizing:border-box}.preview-app{--p-bg:var(--kc-bg);--p-panel:var(--kc-panel);--p-text:var(--kc-ink);--p-muted:var(--kc-muted);--p-line:var(--kc-line);--p-accent:var(--kc-accent-strong);--p-tint:var(--kc-accent-soft);min-height:100vh;background:var(--p-bg);color:var(--p-text);font-family:var(--kc-font)}.preview-nav{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:19px max(24px,calc((100vw - 1240px)/2));background:var(--p-panel);border-bottom:1px solid var(--p-line);font-size:12px}.preview-brand{display:flex;align-items:center;gap:9px;font-weight:650;font-size:19px;text-decoration:none;color:var(--p-text)}.preview-brand>span:first-child{background:var(--kc-action);color:var(--kc-on-action);width:30px;height:30px;border-radius:9px;display:grid;place-items:center;font-size:20px}.preview-brand .brand-sub{font-size:11px;font-weight:400;letter-spacing:1px;color:var(--p-muted)}.preview-nav>div{display:flex;gap:24px}.preview-nav button{cursor:pointer;color:var(--p-muted);font-size:12px}.preview-nav button[aria-pressed=true]{color:var(--p-accent);font-weight:600}.theme-button{border:1px solid var(--p-line);padding:6px 10px;border-radius:7px}.preview-app>main{max-width:1240px;margin:0 auto;padding:32px 24px 60px}.canvas-preview{min-height:780px;border:1px solid var(--p-line);border-radius:16px;padding:35px;background-image:radial-gradient(var(--p-line) 1px,transparent 1px);background-size:18px 18px}.canvas-intro span{font-size:11px;color:var(--p-muted)}.canvas-intro h1{font-size:25px;font-weight:600;margin:15px 0 10px}.canvas-intro p{font-size:12px;color:var(--p-muted)}.picker-preview{background:var(--p-panel);border:1px solid var(--p-line);border-radius:14px;padding:22px;width:min(540px,100%);margin:35px auto;box-shadow:var(--kc-shadow)}.picker-preview header{display:flex;align-items:center;justify-content:space-between;font-size:13px}.picker-preview header span{font-size:10px;color:var(--p-muted)}.picker-tabs{display:flex;gap:8px;margin:17px 0}.picker-tabs button{padding:6px 14px;border:1px solid var(--p-line);border-radius:6px;font-size:12px}.picker-tabs button[aria-pressed=true]{color:var(--p-accent);background:var(--p-tint)}.picker-preview>input{width:100%;background:var(--p-bg);border:1px solid var(--p-line);border-radius:7px;padding:10px;font-size:12px;color:var(--p-text)}.picker-list{margin-top:10px;max-height:270px;overflow:auto}.picker-list button{display:flex;align-items:flex-start;flex-direction:column;width:100%;padding:11px 9px;border-radius:7px;text-align:left;cursor:pointer;color:var(--p-text)}.picker-list button[aria-pressed=true],.picker-list button:hover{background:var(--p-tint)}.picker-list strong{font-size:12px;overflow-wrap:anywhere}.picker-list small{font-size:10px;color:var(--p-muted);margin:3px 0}.picker-list span{font-size:12px;color:var(--p-accent)}.picker-selected{border-top:1px solid var(--p-line);margin-top:12px;padding-top:14px;display:flex;flex-direction:column;gap:8px;font-size:12px}.picker-selected strong{color:var(--p-accent);font-size:20px}.picker-selected button{color:var(--p-accent);font-size:12px;text-align:left;cursor:pointer}.picker-footnote{font-size:10px;color:var(--p-muted);margin:15px 0 0}@media(max-width:600px){.preview-nav{gap:12px;padding:14px}.preview-brand .brand-sub{display:none}.preview-nav>div{gap:12px}.preview-nav button{font-size:10px}.preview-app>main{padding:20px 16px 40px}.canvas-preview{padding:20px 14px}.preview-brand{font-size:16px}.theme-button{padding:5px}}
</style>
