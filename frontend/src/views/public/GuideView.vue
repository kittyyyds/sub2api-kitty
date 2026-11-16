<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { getPublicGuide, type PublicGuide } from '@/api/guide'
import { buildGuideDocumentFromChapters, splitGuideIntoChapters } from '@/utils/guideMarkdown'
import { getBundledGuideChapters } from '@/utils/guideSections'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/composables/useTheme'

const app = useAppStore()
const auth = useAuthStore()
const { isDark, toggleTheme } = useTheme()
const guide = ref<PublicGuide | null>(null)
const loading = ref(true)
const failed = ref(false)
const chapters = computed(() => guide.value?.has_custom_content
  ? (guide.value.chapters?.length ? guide.value.chapters : splitGuideIntoChapters(guide.value.content || ''))
  : getBundledGuideChapters())
const document = computed(() => buildGuideDocumentFromChapters(chapters.value))
async function load() {
  loading.value = true
  failed.value = false
  try { guide.value = await getPublicGuide() } catch { failed.value = true } finally { loading.value = false }
}
onMounted(load)
</script>

<template>
  <div class="guide-page" data-testid="guide-view">
    <header class="guide-header">
      <RouterLink to="/home" class="guide-brand">{{ app.siteName || 'KittyAPI' }}<span>使用教程</span></RouterLink>
      <div class="flex items-center gap-3">
        <button type="button" class="btn btn-secondary btn-sm" @click="toggleTheme">{{ isDark ? '浅色' : '深色' }}</button>
        <RouterLink :to="auth.isAuthenticated ? '/dashboard' : '/login'" class="btn btn-primary btn-sm">{{ auth.isAuthenticated ? '控制台' : '登录' }}</RouterLink>
      </div>
    </header>
    <div class="guide-intro">
      <p class="text-sm font-semibold text-primary-600 dark:text-primary-300">KITTY · 使用帮助</p>
      <h1>使用教程</h1>
      <p>接入步骤、常见问题与站点使用说明。</p>
      <p v-if="guide?.updated_at" class="text-xs">第 {{ guide.version }} 版 · 更新于 {{ new Date(guide.updated_at).toLocaleString() }}</p>
    </div>
    <main class="guide-layout">
      <p v-if="loading" role="status">正在加载教程…</p>
      <div v-else-if="failed" role="alert" class="card p-6">教程暂时无法加载。<button type="button" class="ml-3 text-primary-600 underline" @click="load">重试</button></div>
      <template v-else>
        <nav class="guide-nav" aria-label="教程目录">
          <span class="text-xs font-semibold text-gray-500">文章目录</span>
          <a v-for="section in document.sections" :key="section.id" :href="`#${section.id}`">{{ section.label }}</a>
        </nav>
        <!-- HTML is sanitized by guideMarkdown before rendering. -->
        <article class="guide-content card" data-testid="guide-content" v-html="document.html"></article>
      </template>
    </main>
  </div>
</template>

<style scoped>
.guide-page { min-height:100vh; background:var(--color-background, #f2f1ec); color:#272a25; }
.guide-header { display:flex; align-items:center; justify-content:space-between; gap:1rem; border-bottom:1px solid #dedfd8; padding:1rem max(1.25rem, calc((100vw - 1200px)/2)); background:rgba(255,255,255,.85); }
.guide-brand { font-size:1.15rem; font-weight:700; white-space:nowrap; }
.guide-brand span { margin-left:1rem; padding-left:1rem; border-left:1px solid #d8dad1; color:#777b70; font-size:.85rem; font-weight:500; }
.guide-intro { max-width:1200px; margin:auto; padding:2.5rem 1.25rem 1.5rem; }
.guide-intro h1 { font-size:2.3rem; font-weight:750; margin:.4rem 0; }
.guide-intro p { color:#777b70; margin:.5rem 0; }
.guide-layout { display:grid; grid-template-columns:210px minmax(0,1fr); gap:2rem; max-width:1200px; margin:auto; padding:0 1.25rem 4rem; align-items:start; }
.guide-nav { position:sticky; top:1.5rem; display:flex; flex-direction:column; gap:.5rem; max-height:80vh; overflow:auto; }
.guide-nav a { padding:.5rem .7rem; border-radius:.5rem; font-size:.9rem; }
.guide-nav a:hover { background:#e9e7de; color:#c45100; }
.guide-content { padding:1.5rem 2rem; min-width:0; overflow-wrap:anywhere; line-height:1.85; }
.guide-content :deep(h2) { font-size:1.5rem; font-weight:700; border-bottom:1px solid #e1e3da; padding-bottom:.5rem; margin:2rem 0 1rem; scroll-margin-top:1rem; }
.guide-content :deep(h2:first-child) { margin-top:0; }
.guide-content :deep(h3) { font-size:1.15rem; font-weight:650; margin:1rem 0 .5rem; }
.guide-content :deep(p), .guide-content :deep(ul), .guide-content :deep(ol) { margin:.8rem 0; }
.guide-content :deep(ul) { list-style:disc; padding-left:1.5rem; }
.guide-content :deep(ol) { list-style:decimal; padding-left:1.5rem; }
.guide-content :deep(a) { color:#c45100; text-decoration:underline; text-underline-offset:3px; }
.guide-content :deep(pre) { background:#f2f1ec; border:1px solid #e2e3dc; padding:1rem; border-radius:.6rem; overflow:auto; line-height:1.6; }
.guide-content :deep(code) { font-size:.87em; }
.guide-content :deep(blockquote) { border-left:3px solid #d6a77c; padding:.5rem 1rem; color:#777b70; }
.guide-content :deep(img) { max-width:100%; height:auto; border-radius:.5rem; }
.guide-content :deep(table) { display:block; overflow:auto; border-collapse:collapse; }
.guide-content :deep(th), .guide-content :deep(td) { border:1px solid #dedfd8; padding:.4rem .7rem; }
:global(.dark) .guide-page { background:#161715; color:#e7e9df; }
:global(.dark) .guide-header { background:#20221f; border-color:#363a31; }
:global(.dark) .guide-nav a:hover { background:#292c26; color:#e9a67b; }
:global(.dark) .guide-content :deep(h2), :global(.dark) .guide-content :deep(th), :global(.dark) .guide-content :deep(td) { border-color:#363a31; }
:global(.dark) .guide-content :deep(a) { color:#e9a67b; }
:global(.dark) .guide-content :deep(pre) { background:#292c26; border-color:#363a31; }
@media(max-width:767px) { .guide-layout { grid-template-columns:minmax(0,1fr); gap:1rem; } .guide-nav { position:static; max-height:14rem; border-left:2px solid #dadcd2; padding-left:.5rem; } .guide-content { padding:1.25rem; } .guide-brand span { display:none; } }
</style>
