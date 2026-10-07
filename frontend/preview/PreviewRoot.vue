<template>
  <div class="preview-toolbar"><strong>本地预览 · 示例数据</strong><nav><RouterLink to="/dashboard">首页</RouterLink><RouterLink to="/monitor">渠道状态 V1</RouterLink><button type="button" @click="toggleBrand">{{ brand }} ⇄</button><button type="button" @click="toggleTheme">深 / 浅色</button></nav><select :value="scenario" aria-label="预览场景" @change="changeScenario"><option value="normal">正常状态</option><option value="empty">暂无公告 / 渠道</option><option value="error">加载失败</option><option value="read-error">已读保存失败</option><option value="simple">简易模式</option></select></div>
  <RouterView />
  <Toast />
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import Toast from '@/components/common/Toast.vue'
import { scenario } from './fixtures'
const brand = ref('Kitty奇妙屋'), store = useAppStore()
function toggleBrand() { brand.value = brand.value === 'Kitty奇妙屋' ? 'Kitty API' : 'Kitty奇妙屋'; store.siteName = brand.value; if (store.cachedPublicSettings) { store.cachedPublicSettings.site_name = brand.value; store.cachedPublicSettings.api_base_url = brand.value === 'Kitty API' ? 'https://kittyapi.xyz' : 'https://kittyyyds.xyz' } }
function toggleTheme() { document.documentElement.classList.toggle('dark') }
function changeScenario(event: Event) { const url = new URL(location.href); url.searchParams.set('scenario', (event.target as HTMLSelectElement).value); location.href = url.href }
</script>
<style>
.preview-toolbar { position: relative; z-index: 45; min-height: 38px; display: flex; align-items: center; justify-content: center; gap: 24px; flex-wrap: wrap; padding: 8px 16px; background: #233a32; color: #eff8ee; font: 11px 'Microsoft YaHei',sans-serif; }
.preview-toolbar strong { font-weight: 500; opacity: .8; }.preview-toolbar nav { display: flex; gap: 18px; }.preview-toolbar .router-link-active { color: #f2c5a0; }.preview-toolbar select { background: #304b40; color: #eff8ee; border: 1px solid #ffffff30; border-radius: 5px; font-size: 10px; padding: 3px 5px; }
.kitty-customer-shell .sidebar { top: 38px; height: calc(100dvh - 38px); }
@media(max-width:700px) { .preview-toolbar { gap: 10px; } .preview-toolbar select { display: none; } }
</style>
