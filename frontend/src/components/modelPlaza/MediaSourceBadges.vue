<script setup lang="ts">
import type { MediaCatalogModel } from './mediaCatalogPolicy'
defineProps<{ document?: MediaCatalogModel; showEmpty?: boolean }>()
// Color only: labels are copied from the source, never inferred from a model ID.
function tone(label: string) {
  if (/下架|不能用|不可用|不稳定|失败|缺号|维护|无法使用/.test(label)) return 'warning'
  if (/凌晨|夜间|白天|时段/.test(label)) return 'timing'
  if (/特惠|特价|福利|优惠/.test(label)) return 'offer'
  if (/新增|新品/.test(label)) return 'new'
  if (/推荐|可用|稳定|成功率|快/.test(label)) return 'positive'
  return 'neutral'
}
</script>

<template>
  <div v-if="document?.sourceBadges?.length || document?.sourceRecommendation || showEmpty" class="source-annotations" data-testid="source-annotations" aria-label="价表选型标注（非实时检测）">
    <div v-if="document?.sourceBadges?.length" class="source-badges">
      <span v-for="label in document.sourceBadges" :key="label" class="source-badge" :class="'badge-' + tone(label)" data-testid="source-badge">{{ label }}</span>
    </div>
    <div v-if="document?.sourceRecommendation" class="source-recommendation"><small>推荐值</small><span>{{ document.sourceRecommendation }}</span></div>
    <small v-if="!document?.sourceBadges?.length && !document?.sourceRecommendation && showEmpty" class="source-empty">未标注</small>
  </div>
</template>

<style scoped>
.source-annotations{margin:0 0 10px}.source-badges{display:flex;gap:6px;flex-wrap:wrap}.source-badge{display:inline-flex;align-items:center;padding:5px 8px;border-radius:6px;border:1px solid var(--badge-line,var(--mc-line));background:var(--badge-bg,var(--mc-soft));color:var(--badge-ink,var(--mc-ink));font-size:11px;font-weight:650;line-height:1.5;white-space:pre-line;overflow-wrap:anywhere}.badge-positive{--badge-ink:#11624f;--badge-bg:#e5f4ec;--badge-line:#b9decf}.badge-offer{--badge-ink:#a42c47;--badge-bg:#fff0f2;--badge-line:#f2c4ce}.badge-warning{--badge-ink:#8b4c00;--badge-bg:#fff2d5;--badge-line:#e9cb8d}.badge-timing{--badge-ink:#315f99;--badge-bg:#eaf1fe;--badge-line:#c0d3f1}.badge-new{--badge-ink:#70509d;--badge-bg:#f2ebfc;--badge-line:#d5c2ed}.source-recommendation{display:flex;gap:5px;flex-wrap:wrap;align-items:center;margin-top:8px;color:var(--mc-ink);font-size:12px}.source-recommendation small,.source-empty{color:var(--mc-muted);font-size:10px}.source-recommendation span{overflow-wrap:anywhere}:global(.media-catalog.is-dark .badge-positive){--badge-ink:#9ce2c0;--badge-bg:#1b3c36;--badge-line:#345e50}:global(.media-catalog.is-dark .badge-offer){--badge-ink:#ffc3cd;--badge-bg:#492b36;--badge-line:#734553}:global(.media-catalog.is-dark .badge-warning){--badge-ink:#ffe0a6;--badge-bg:#463820;--badge-line:#78613b}:global(.media-catalog.is-dark .badge-timing){--badge-ink:#c2d6ff;--badge-bg:#233b56;--badge-line:#456184}:global(.media-catalog.is-dark .badge-new){--badge-ink:#e2cbff;--badge-bg:#392f51;--badge-line:#625178}
</style>
