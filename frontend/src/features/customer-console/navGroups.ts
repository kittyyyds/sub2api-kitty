import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { FeatureFlags, resolveFeatureFlag, type FeatureFlagDefinition } from '@/utils/featureFlags'

// Related customer pages share one sidebar entry and switch through tabs at the
// top of the page. Routes, guards and the pages themselves stay as they are.
export interface CustomerNavGroupMember {
  path: string
  labelKey: string
  /** Read on use, not on import, like the sidebar's own flags. */
  flag?: () => FeatureFlagDefinition
  hideInSimpleMode?: boolean
  /** The page has its own layout outside the console, so the tab leaves the console. */
  standalone?: boolean
}

export interface CustomerNavGroup {
  id: string
  labelKey: string
  members: CustomerNavGroupMember[]
}

export const CUSTOMER_NAV_GROUPS: CustomerNavGroup[] = [
  {
    id: 'channels',
    labelKey: 'nav.channelsGroup',
    // Status first: it is what the sidebar entry opens.
    members: [
      { path: '/monitor', labelKey: 'nav.channelStatus', flag: () => FeatureFlags.channelMonitor },
      { path: '/available-channels', labelKey: 'nav.availableChannels', flag: () => FeatureFlags.availableChannels, hideInSimpleMode: true },
    ],
  },
  {
    id: 'docs',
    labelKey: 'nav.usageDocs',
    members: [
      { path: '/custom/docs', labelKey: 'nav.usageDocs' },
      { path: '/guide', labelKey: 'admin.settings.tabs.guide', standalone: true },
    ],
  },
  {
    id: 'billing-records',
    labelKey: 'nav.billingRecordsGroup',
    members: [
      { path: '/billing', labelKey: 'nav.billing', hideInSimpleMode: true },
      { path: '/orders', labelKey: 'nav.myOrders', flag: () => FeatureFlags.payment, hideInSimpleMode: true },
      { path: '/invoices', labelKey: 'invoice.title', flag: () => FeatureFlags.payment, hideInSimpleMode: true },
    ],
  },
  {
    id: 'recharge-center',
    labelKey: 'nav.rechargeCenterGroup',
    members: [
      { path: '/purchase', labelKey: 'nav.recharge', flag: () => FeatureFlags.payment, hideInSimpleMode: true },
      { path: '/redeem', labelKey: 'nav.redeem', hideInSimpleMode: true },
      { path: '/coupons', labelKey: 'coupon.title', hideInSimpleMode: true },
    ],
  },
]

/**
 * Collapses each group to its first visible member. `items` must already be
 * filtered for visibility; a group with a single visible member is left alone.
 */
export function mergeCustomerNavGroups<T extends { path: string; label: string }>(
  items: T[],
  groupLabel: (group: CustomerNavGroup) => string,
): Array<T & { activePaths?: string[] }> {
  const merged: Array<T & { activePaths?: string[] }> = [...items]
  for (const group of CUSTOMER_NAV_GROUPS) {
    const present = group.members.map(member => member.path).filter(path => merged.some(item => item.path === path))
    if (present.length < 2) continue
    const [lead, ...rest] = present
    const index = merged.findIndex(item => item.path === lead)
    merged[index] = { ...merged[index], label: groupLabel(group), activePaths: present }
    for (const path of rest) merged.splice(merged.findIndex(item => item.path === path), 1)
  }
  return merged
}

/** Tabs for the group the current route belongs to; empty when the page stands alone. */
export function useCustomerPageTabs() {
  const route = useRoute()
  const appStore = useAppStore()
  const authStore = useAuthStore()
  return computed(() => {
    const group = CUSTOMER_NAV_GROUPS.find(candidate => candidate.members.some(member => route.path === member.path || route.path.startsWith(member.path + '/')))
    if (!group) return []
    const visible = group.members.filter(member =>
      (!member.flag || resolveFeatureFlag(appStore.cachedPublicSettings, member.flag()))
      && !(member.hideInSimpleMode && authStore.isSimpleMode))
    return visible.length > 1 ? visible : []
  })
}
