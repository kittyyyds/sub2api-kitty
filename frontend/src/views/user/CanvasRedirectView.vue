<script setup lang="ts">
import { onMounted } from 'vue'
import { useAppStore } from '@/stores/app'
import { getCreativeCanvasUrl } from '@/utils/creativeCanvas'
const appStore = useAppStore()
const destination = getCreativeCanvasUrl(appStore.siteVersion)
onMounted(() => window.location.replace(destination))
</script>

<template>
  <main class="p-8 text-center" aria-live="polite">
    <h1 class="text-xl font-semibold">正在进入无限画布…</h1>
    <p class="mt-3">图片生成、视频创作已统一迁入无限画布。</p>
    <a :href="destination" class="btn btn-primary mt-5">进入无限画布 · 生图与视频</a>
  </main>
</template>
