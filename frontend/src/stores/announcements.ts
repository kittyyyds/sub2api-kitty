import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { announcementsAPI } from '@/api'
import type { UserAnnouncement } from '@/types'

const THROTTLE_MS = 20 * 60 * 1000 // 20 minutes

export const useAnnouncementStore = defineStore('announcements', () => {
  // State
  const announcements = ref<UserAnnouncement[]>([])
  const loading = ref(false)
  const fetchError = ref(false)
  const lastFetchTime = ref(0)
  const popupQueue = ref<UserAnnouncement[]>([])
  const currentPopup = ref<UserAnnouncement | null>(null)

  // Session-scoped dedup set — not reactive, used as plain lookup only
  let shownPopupIds = new Set<number>()
  let fetchGeneration = 0

  // Getters
  const unreadCount = computed(() =>
    announcements.value.filter((a) => !a.read_at).length
  )

  // Actions
  async function fetchAnnouncements(force = false) {
    const now = Date.now()
    if (!force && lastFetchTime.value > 0 && now - lastFetchTime.value < THROTTLE_MS) {
      return
    }

    // Set immediately to prevent concurrent duplicate requests
    lastFetchTime.value = now
    const generation = ++fetchGeneration

    try {
      loading.value = true
      fetchError.value = false
      const all = await announcementsAPI.list(false)
      if (generation !== fetchGeneration) return
      announcements.value = all
      enqueueNewPopups()
    } catch (err: any) {
      if (generation !== fetchGeneration) return
      // Revert throttle timestamp on failure so retry is allowed
      lastFetchTime.value = 0
      fetchError.value = true
      console.error('Failed to fetch announcements:', err)
    } finally {
      if (generation === fetchGeneration) loading.value = false
    }
  }

  function enqueueNewPopups() {
    const newPopups = announcements.value.slice(0, 20).filter(
      (a) => a.notify_mode === 'popup' && !a.read_at && !shownPopupIds.has(a.id)
    )
    if (newPopups.length === 0) return

    for (const p of newPopups) {
      if (!popupQueue.value.some((q) => q.id === p.id)) {
        popupQueue.value.push(p)
      }
    }

    if (!currentPopup.value) {
      showNextPopup()
    }
  }

  function showNextPopup() {
    if (popupQueue.value.length === 0) {
      currentPopup.value = null
      return
    }
    currentPopup.value = popupQueue.value.shift()!
    shownPopupIds.add(currentPopup.value.id)
  }

  async function dismissPopup() {
    if (!currentPopup.value) return
    const id = currentPopup.value.id
    currentPopup.value = null

    // Mark as read (fire-and-forget, UI already updated)
    markAsRead(id)

    // Show next popup after a short delay
    if (popupQueue.value.length > 0) {
      setTimeout(() => showNextPopup(), 300)
    }
  }

  async function markAsRead(id: number) {
    const generation = fetchGeneration
    try {
      await announcementsAPI.markRead(id)
      if (generation !== fetchGeneration) return false
      const ann = announcements.value.find((a) => a.id === id)
      if (ann) {
        ann.read_at = new Date().toISOString()
      }
      return true
    } catch (err: any) {
      console.error('Failed to mark announcement as read:', err)
      return false
    }
  }

  async function markAllAsRead() {
    const generation = fetchGeneration
    const unread = announcements.value.filter((a) => !a.read_at)
    if (unread.length === 0) return

    try {
      loading.value = true
      // The history list is no longer capped at 20. Bound concurrent writes
      // rather than dispatching one request per historical notice at once.
      let failure: PromiseRejectedResult | undefined
      for (let offset = 0; offset < unread.length; offset += 5) {
        if (generation !== fetchGeneration) return
        const results = await Promise.allSettled(unread.slice(offset, offset + 5).map(async (a) => {
          await announcementsAPI.markRead(a.id)
          a.read_at = new Date().toISOString()
        }))
        failure ??= results.find((result): result is PromiseRejectedResult => result.status === 'rejected')
      }
      if (failure) throw failure.reason
    } catch (err: any) {
      console.error('Failed to mark all as read:', err)
      throw err
    } finally {
      if (generation === fetchGeneration) loading.value = false
    }
  }

  function reset() {
    fetchGeneration++
    announcements.value = []
    lastFetchTime.value = 0
    shownPopupIds = new Set()
    popupQueue.value = []
    currentPopup.value = null
    loading.value = false
    fetchError.value = false
  }

  return {
    // State
    announcements,
    loading,
    fetchError,
    currentPopup,
    // Getters
    unreadCount,
    // Actions
    fetchAnnouncements,
    dismissPopup,
    markAsRead,
    markAllAsRead,
    reset,
  }
})
