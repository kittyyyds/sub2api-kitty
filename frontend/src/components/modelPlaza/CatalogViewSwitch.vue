<script setup lang="ts">
defineProps<{ modelValue: 'all' | 'media' }>()
defineEmits<{ 'update:modelValue': [value: 'all' | 'media'] }>()
</script>

<template>
  <nav class="catalog-switch" aria-label="选择价格目录">
    <button v-for="option in ([{ id: 'all', icon: '💬', title: '全部模型价格', subtitle: '文本模型 · Grok 视频 · 分组价格' }, { id: 'media', icon: '🎨', title: '图片 / 视频模型价格', subtitle: '创作模型 · 规格说明 · 费用试算' }] as const)"
      :key="option.id" type="button" :aria-pressed="modelValue === option.id"
      @click="$emit('update:modelValue', option.id)">
      <span class="catalog-switch-icon" aria-hidden="true">{{ option.icon }}</span>
      <span class="catalog-switch-copy"><strong>{{ option.title }}</strong><small>{{ option.subtitle }}</small></span>
      <span class="catalog-switch-action">{{ modelValue === option.id ? '正在查看 ✓' : '点击查看 →' }}</span>
    </button>
  </nav>
</template>

<style scoped>
.catalog-switch { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 14px; margin-bottom: 22px; color: var(--kc-ink,#20231e); }
.catalog-switch button { display: flex; align-items: center; gap: 12px; min-height: 88px; padding: 18px 20px; border: 1.5px solid var(--kc-line,#ded8ce); border-radius: 14px; background: var(--kc-panel,#fff); text-align: left; cursor: pointer; transition: border-color .15s,box-shadow .15s; }
.catalog-switch button:hover { border-color: var(--kc-accent-strong,#a54608); box-shadow: 0 3px 12px #a5460814; }
.catalog-switch button:focus-visible { outline: 3px solid var(--kc-accent-strong,#a54608); outline-offset: 3px; }
.catalog-switch button[aria-pressed='true'] { border-color: var(--kc-accent-strong,#a54608); background: var(--kc-accent-soft,#fff5ed); box-shadow: inset 0 0 0 1px var(--kc-accent-strong,#a54608); }
.catalog-switch-icon { display: grid; place-items: center; flex-shrink: 0; width: 40px; height: 40px; border-radius: 12px; background: var(--kc-bg,#f5f3ed); font-size: 24px; }
.catalog-switch-copy { flex: 1; min-width: 0; }
.catalog-switch strong { display: block; font-size: 17px; font-weight: 700; line-height: 1.5; }
.catalog-switch small { display: block; margin-top: 4px; color: var(--kc-muted,#65665f); font-size: 12px; line-height: 1.5; }
.catalog-switch-action { flex-shrink: 0; padding: 6px 9px; border-radius: 7px; background: var(--kc-bg,#f5f3ed); color: var(--kc-accent-strong,#a54608); font-size: 12px; font-weight: 600; }
.catalog-switch button[aria-pressed='true'] .catalog-switch-action { background: var(--kc-accent-strong,#a54608); color: var(--kc-panel,#fff); }
@media (max-width: 900px) { .catalog-switch button { padding: 14px; gap: 9px; } .catalog-switch strong { font-size: 15px; } .catalog-switch-action { font-size: 11px; } }
@media (max-width: 640px) { .catalog-switch { grid-template-columns: 1fr; gap: 10px; } .catalog-switch button { min-height: 78px; } }
</style>
