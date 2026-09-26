import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const src = (relative: string) => readFileSync(resolve(here, relative), 'utf8')

const dashboardView = src('../DashboardView.vue')
const stats = src('../../../components/user/dashboard/UserDashboardStats.vue')
const charts = src('../../../components/user/dashboard/UserDashboardCharts.vue')
const recent = src('../../../components/user/dashboard/UserDashboardRecentUsage.vue')
const quickActions = src('../../../components/user/dashboard/UserDashboardQuickActions.vue')
const layout = src('../../../components/layout/AppLayout.vue')
const header = src('../../../components/layout/AppHeader.vue')
const theme = src('../../../styles/console-theme.css')
const tokenTrend = src('../../../components/charts/TokenUsageTrend.vue')

const requiredDashboardSections = [
  'UserDashboardStats',
  'UserDashboardCharts',
  'UserDashboardRecentUsage',
  'UserDashboardQuickActions',
]

describe('native customer dashboard skin contract', () => {
  it('keeps every complete dashboard feature in the native view', () => {
    for (const section of requiredDashboardSections) {
      expect(dashboardView).toContain(section)
    }
    expect(dashboardView).toContain('dashboard-view-content')
    expect(dashboardView).toContain('dashboard-recent-grid')
  })

  it('exposes screenshot-matched layout hooks on native dashboard components', () => {
    expect(stats).toContain('data-dashboard-stats')
    expect(stats).toContain('dashboard-stats-row')
    expect(stats).toContain('dashboard-platform-section')
    expect(stats).toContain('dashboard-platform-grid')
    expect(stats).toContain("platformCount', { count: platformCount }")
    expect(stats).toContain("grok: 'Grok'")
    expect(charts).toContain('dashboard-filter-bar')
    expect(charts).toContain('dashboard-charts-grid')
    expect(charts).toContain('dashboard-chart-panel')
    expect(recent).toContain('dashboard-recent-usage')
    expect(quickActions).toContain('dashboard-quick-actions')
  })

  it('uses the spacious console shell without changing routing behavior', () => {
    expect(layout).toContain('kitty-console-main')
    expect(layout).toContain("sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64'")
    expect(header).toContain('data-console-header')
    expect(header).toContain('data-console-header-actions')
  })

  it('keeps Kitty branding and responsive grids while sharing tokens with portals', () => {
    expect(theme).toContain('--kc-bg: #f2f1ec')
    expect(theme).toContain('--kc-accent: #c45100')
    expect(theme).toContain('--kc-panel: #202b3b')
    expect(theme).toContain('html.kitty-console-theme')
    expect(theme).toContain('.dashboard-charts-grid')
    expect(theme).toContain('repeat(2, minmax(0, 1fr))')
    expect(theme).toContain('repeat(4, minmax(0, 1fr))')
    expect(theme).toContain('@media (max-width: 1199px)')
  })

  it('turns off chart animation in the detected lite mode', () => {
    expect(tokenTrend).toContain('detectConsolePerformanceMode')
    expect(tokenTrend).toContain('animation: performanceMode === \'lite\' ? false')
  })
})
