import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import enCommon from '@/i18n/locales/en/common'
import zhCommon from '@/i18n/locales/zh/common'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar theme integration', () => {
  it('uses the shared theme composable instead of maintaining a second DOM theme state', () => {
    expect(componentSource).toContain("import { useTheme } from '@/composables/useTheme'")
    expect(componentSource).toContain('const { isDark, toggleTheme } = useTheme()')
    expect(componentSource).not.toContain("const isDark = ref(document.documentElement.classList.contains('dark'))")
    expect(componentSource).not.toContain("localStorage.setItem('theme'")
    expect(componentSource).not.toContain("const savedTheme = localStorage.getItem('theme')")
  })
})

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar regular-user flat navigation contract', () => {
  it('uses the Kitty Studio brand and keeps chat as a first-class entry', () => {
    expect(zhCommon.nav.appCenter).toBe('Kitty生图')
    expect(enCommon.nav.appCenter).toBe('Kitty Studio')
    expect(zhCommon.appCenter.title).toBe('Kitty生图')
    expect(enCommon.appCenter.title).toBe('Kitty Studio')
    expect(zhCommon.nav.onlineChat).toBe('在线聊天')
    expect(enCommon.nav.onlineChat).toBe('Online Chat')
    expect(zhCommon.nav).not.toHaveProperty('kittyImage')
    expect(enCommon.nav).not.toHaveProperty('kittyImage')
  })

  it('renders direct links and keeps the API-key onboarding selector on its link', () => {
    expect(componentSource).toContain('v-for="item in userNavItems"')
    expect(componentSource).toContain(':to="item.path"')
    expect(componentSource).toContain(":data-tour=\"item.path === '/keys' ? 'sidebar-my-keys' : undefined\"")
    expect(componentSource).toContain('@click="handleMenuItemClick(item.path)"')
    expect(componentSource).not.toContain('data-user-nav-group')
  })

  it('renders bright promotion badges inside their flat navigation links', () => {
    expect(componentSource).toContain(':data-sidebar-promo="item.badge.tone"')
    expect(componentSource).toContain("label: t('nav.hotRecommendation')")
    expect(componentSource).toContain("label: t('nav.limitedOffer')")
    expect(componentSource).toContain("tone: 'deal'")
    expect(componentSource).toContain("label: t('nav.affiliateBadge')")
    expect(componentSource).toContain("tone: 'rebate'")
    expect(componentSource).toContain("path: '/apps'")
    expect(componentSource).toContain("path: '/chat'")
    expect(componentSource).not.toContain("buildNativeFeatureNavItem('image-generator')")
    expect(componentSource).not.toContain("buildNativeFeatureNavItem('reset-radar')")
    expect(componentSource).not.toContain("path: '/model-plaza?embedded=1'")
  })

  it('keeps mobile close behavior without customer-only group state', () => {
    expect(componentSource).toContain('appStore.setMobileOpen(false)')
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('groupUserNavItems')
    expect(componentSource).not.toContain('collapsedPath')
  })

  it('filters the flat customer routes without changing the admin personal menu source', () => {
    expect(componentSource).toContain('const userNavItems = computed((): NavItem[] => finalizeNav(buildSelfNavItems(true)))')
    expect(componentSource).toContain('const personalNavItems = computed((): NavItem[] => finalizeNav(buildSelfNavItems(false)))')
  })
})

describe('AppSidebar subscription feature flag', () => {
  it('gates the My Subscriptions entry behind the subscription public-settings flag', () => {
    expect(componentSource).toContain('const flagSubscription = makeSidebarFlag(FeatureFlags.subscription)')
    expect(componentSource).toMatch(/path: '\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('also hides the admin Subscription Management entry on recharge-only sites', () => {
    expect(componentSource).toMatch(/path: '\/admin\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('derives the purchase entry label from the site billing mode', () => {
    expect(componentSource).toContain("import { resolveSiteBillingMode } from '@/utils/siteBillingMode'")
    expect(componentSource).toMatch(/case 'recharge_only':\s*return t\('nav\.recharge'\)/)
    expect(componentSource).toMatch(/case 'subscription_only':\s*return t\('nav\.subscribe'\)/)
    expect(componentSource).toMatch(/path: '\/purchase',\s*label: purchaseNavLabel\.value/)
  })
})
