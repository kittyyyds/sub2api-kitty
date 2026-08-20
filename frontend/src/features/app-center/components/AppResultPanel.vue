<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

import type { LocalMediaResult } from '../types'
import DisabledVideoPreview from './DisabledVideoPreview.vue'

defineProps<{ result: LocalMediaResult | null; running: boolean }>()
</script>

<template>
  <section class="result-panel">
    <header><div><Icon name="sparkles" size="sm" /><strong>当前结果</strong></div><span>{{ running ? '处理中' : result ? '已生成' : '待命' }}</span></header>
    <div v-if="running && !result" class="result-empty"><span class="loader" /><p>执行当前编排步骤</p></div>
    <div v-else-if="result?.type === 'images' && result.images?.length" class="image-grid">
      <figure v-for="(image, index) in result.images" :key="`${image.url}-${index}`"><img :src="image.url" :alt="`生成图片 ${index + 1}`"><figcaption><span>{{ image.revisedPrompt || `图片 ${index + 1}` }}</span><a :href="image.url" :download="`app-result-${index + 1}.png`" target="_blank" rel="noopener noreferrer" title="下载"><Icon name="download" size="sm" /></a></figcaption></figure>
    </div>
    <div v-else-if="result?.type === 'video' && result.videoUrl" class="video-result">
      <DisabledVideoPreview label="在线播放已关闭，请下载后查看" />
      <div class="video-result-meta"><code>{{ result.requestId }}</code><a :href="result.videoUrl" download="app-video.mp4"><Icon name="download" size="sm" />下载</a></div>
    </div>
    <div v-else-if="result?.type === 'planner'" class="planner-result"><pre>{{ JSON.stringify(result.plannerOutput, null, 2) }}</pre></div>
    <div v-else class="result-empty"><Icon name="inbox" size="lg" /><p>运行结果会显示在这里</p></div>
  </section>
</template>

<style scoped>
.result-panel { min-height: 210px; overflow: hidden; border: 1px solid var(--customer-line); border-radius: 7px; background: var(--customer-surface); }
header { display: flex; min-height: 52px; align-items: center; justify-content: space-between; padding: 11px 14px; border-bottom: 1px solid var(--customer-line); }
header div { display: flex; align-items: center; gap: 7px; color: var(--customer-copper); }
header strong { color: var(--customer-ink); font-size: 12px; }
header span { color: var(--customer-cyan); font-size: 10px; font-weight: 700; }
.result-empty { display: flex; min-height: 156px; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: var(--customer-muted); }
.result-empty p { margin: 0; font-size: 11px; }
.loader { width: 28px; height: 28px; border: 2px solid var(--customer-line); border-top-color: var(--customer-cyan); border-radius: 50%; animation: spin .8s linear infinite; }
.image-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; padding: 10px; }
figure { min-width: 0; margin: 0; overflow: hidden; border: 1px solid var(--customer-line); border-radius: 5px; }
figure img { display: block; width: 100%; aspect-ratio: 1; background: var(--customer-surface-soft); object-fit: contain; }
figcaption { display: flex; align-items: center; gap: 8px; padding: 8px; }
figcaption span { flex: 1; overflow: hidden; color: var(--customer-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
figcaption a { color: var(--customer-cyan); }
.video-result { padding: 10px; }
.video-result-meta { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding-top: 9px; }
.video-result code { min-width: 0; overflow: hidden; color: var(--customer-muted); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.video-result a { display: inline-flex; align-items: center; gap: 5px; color: var(--customer-cyan); font-size: 10px; }
.planner-result { max-height: 310px; overflow: auto; padding: 12px; }
.planner-result pre { margin: 0; color: var(--customer-muted); font-size: 10px; line-height: 1.55; white-space: pre-wrap; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .loader { animation: none; } }
</style>
