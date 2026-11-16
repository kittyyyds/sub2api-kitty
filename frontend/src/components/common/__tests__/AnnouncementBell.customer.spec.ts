import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import AnnouncementBell from '../AnnouncementBell.vue'

const announcementStore = reactive({
  announcements: ref([]),
  loading: ref(false),
  unreadCount: 2,
  markAsRead: vi.fn(),
  markAllAsRead: vi.fn(),
})

const appStore = {
  showError: vi.fn(),
  showSuccess: vi.fn(),
}

vi.mock('@/stores/announcements', () => ({
  useAnnouncementStore: () => announcementStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key === 'announcements.title' ? '公告' : key,
    }),
  }
})

describe('AnnouncementBell customer trigger', () => {
  it('shows a prominent labeled announcement button and keeps unread state visible', () => {
    const wrapper = mount(AnnouncementBell, {
      global: {
        stubs: {
          Icon: { props: ['name'], template: '<span :data-icon="name" />' },
          Teleport: true,
          Transition: false,
        },
      },
    })

    const trigger = wrapper.get('button.announcement-header-button')
    expect(trigger.text()).toContain('公告')
    expect(trigger.attributes('aria-label')).toBe('公告')
    expect(trigger.classes()).toContain('announcement-header-button--unread')
    expect(trigger.find('[data-icon="bell"]').exists()).toBe(true)

    wrapper.unmount()
  })
})
