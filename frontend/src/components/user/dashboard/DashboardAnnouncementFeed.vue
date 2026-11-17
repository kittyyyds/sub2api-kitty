<template>
  <section class="home-panel home-announcements" data-testid="home-announcements">
    <header class="home-announcement-header"><div><h2>{{ t('homeOverview.notice') }} <span v-if="store.unreadCount" class="home-unread-count">{{ t('homeOverview.unread', { n: store.unreadCount }) }}</span></h2><p>{{ t('homeOverview.noticeHint') }}</p></div><button v-if="store.unreadCount" type="button" class="home-mark-all" :disabled="marking" @click="markAll">{{ t('homeOverview.markAll') }}</button></header>
    <div v-if="store.fetchError" class="home-error" role="alert">{{ t('homeOverview.noticeError') }} <button type="button" @click="store.fetchAnnouncements(true)">{{ t('homeOverview.retry') }}</button></div>
    <div v-if="markError" class="home-error" role="alert">{{ t('homeOverview.markError') }}</div>
    <div v-if="store.loading && !store.announcements.length" class="home-empty" role="status">{{ t('homeOverview.loading') }}</div>
    <template v-else-if="store.announcements.length">
      <div v-if="searchOpen" class="home-notice-filters"><input v-model="search" type="search" :placeholder="t('homeOverview.search')" :aria-label="t('homeOverview.search')" /><label><input v-model="unreadOnly" type="checkbox" /> {{ t('homeOverview.onlyUnread') }}</label></div>
      <div v-if="!filtered.length" class="home-empty">{{ t('homeOverview.noMatches') }}</div>
      <ol v-else class="home-notice-list">
        <li v-for="notice in pageItems" :key="notice.id"><button type="button" @click="openNotice(notice)"><span class="home-notice-icon" :class="{ 'is-unread': !notice.read_at }"><Icon :name="notice.read_at ? 'checkCircle' : 'bell'" size="sm" /></span><span class="home-history-info"><span class="home-notice-head"><span class="home-notice-title">{{ notice.title }} <small v-if="!notice.read_at">{{ t('homeOverview.unreadBadge') }}</small></span><time :datetime="notice.starts_at || notice.created_at">{{ date(notice.starts_at || notice.created_at) }}</time></span><span class="home-notice-excerpt">{{ excerpt(notice.content) }}</span></span><Icon name="chevronRight" size="sm" /></button></li>
      </ol>
      <footer class="home-history-pagination"><button type="button" :aria-expanded="searchOpen" @click="searchOpen = !searchOpen">{{ t('homeOverview.history') }}</button><span>{{ t('homeOverview.page', { page, total: pages }) }}</span><div><button type="button" :disabled="page <= 1" @click="page--">{{ t('homeOverview.previous') }}</button><button type="button" :disabled="page >= pages" @click="page++">{{ t('homeOverview.next') }}</button></div></footer>
    </template>
    <div v-else-if="!store.fetchError" class="home-empty">{{ t('homeOverview.noNotices') }}</div>
    <BaseDialog :show="!!selected" :title="selected?.title || t('homeOverview.notice')" width="wide" @close="selected = null">
      <template v-if="selected"><time>{{ date(selected.starts_at || selected.created_at) }}</time><div class="announcement-markdown home-notice-dialog" v-html="renderContent(selected.content)"></div></template>
      <template #footer><button v-if="selected && !selected.read_at" type="button" class="btn btn-primary" :disabled="marking" @click="markRead(selected.id)">{{ t('homeOverview.markRead') }}</button><p v-if="markError" role="alert">{{ t('homeOverview.markError') }}</p></template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { useAnnouncementStore } from '@/stores/announcements'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import '@/styles/announcement-markdown.css'
const { t, locale } = useI18n()
const store = useAnnouncementStore()
const search = ref(''), searchOpen = ref(false), unreadOnly = ref(false), page = ref(1), selected = ref<UserAnnouncement | null>(null)
const reading = reactive(new Set<number>()), markingAll = ref(false), markError = ref(false)
const marking = computed(() => markingAll.value || reading.size > 0)
const sorted = computed(() => [...store.announcements].sort((a, b) => new Date(b.starts_at || b.created_at).getTime() - new Date(a.starts_at || a.created_at).getTime() || b.id - a.id))
const filtered = computed(() => sorted.value.filter(a => (!unreadOnly.value || !a.read_at) && `${a.title} ${a.content}`.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / 5)))
const pageItems = computed(() => filtered.value.slice((page.value - 1) * 5, page.value * 5))
watch([search, unreadOnly], () => { page.value = 1 })
watch(pages, n => { page.value = Math.min(page.value, n) })
const date = (value: string) => new Date(value).toLocaleDateString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { year: 'numeric', month: 'short', day: 'numeric' })
const renderContent = (content: string) => DOMPurify.sanitize(marked.parse(content, { async: false, gfm: true, breaks: true }) as string)
function excerpt(content: string) {
  const element = document.createElement('div')
  element.innerHTML = DOMPurify.sanitize(marked.parse(content, { async: false }) as string, { ALLOWED_TAGS: [], ALLOWED_ATTR: [] })
  return (element.textContent || '').replace(/\s+/g, ' ').trim()
}
function openNotice(notice: UserAnnouncement) {
  selected.value = notice
  markError.value = false
  if (!notice.read_at) void markRead(notice.id)
}
async function markRead(id: number) {
  if (reading.has(id)) return
  reading.add(id); markError.value = false
  try {
    const saved = await store.markAsRead(id)
    if (selected.value?.id === id) markError.value = !saved
  } finally { reading.delete(id) }
}
async function markAll() {
  if (marking.value) return
  markingAll.value = true; markError.value = false
  try { await store.markAllAsRead() } catch { markError.value = true } finally { markingAll.value = false }
}
onMounted(() => { void store.fetchAnnouncements() })
</script>
