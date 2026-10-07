import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory, RouterLink } from 'vue-router'
import { apiClient } from '@/api/client'
import i18n, { loadLocaleMessages } from '@/i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { PublicSettings } from '@/types'
import DashboardView from '@/views/user/DashboardView.vue'
import ChannelStatusV1View from '@/views/user/ChannelStatusV1View.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import PreviewRoot from './PreviewRoot.vue'
import { fixtureAdapter, sampleUser, mockCalls } from './fixtures'
import '@/style.css'

if (!import.meta.env.DEV || !['127.0.0.1', 'localhost', '[::1]'].includes(location.hostname)) throw new Error('Preview is restricted to a local development server')
// Every application request resolves in memory. Never proxy preview traffic to
// production, and never use a real account or real session in this entry point.
apiClient.defaults.adapter = fixtureAdapter
localStorage.setItem('user_guide_900001_user_v4_interactive', 'true')
const app = createApp(PreviewRoot), pinia = createPinia()
app.use(pinia)
const auth = useAuthStore(); auth.user = sampleUser; auth.token = 'local-preview-not-a-credential'
const settings = useAppStore()
settings.cachedPublicSettings = { site_name: 'Kitty奇妙屋', site_subtitle: 'AI 模型聚合平台', site_logo: '', api_base_url: 'https://kittyyyds.xyz', payment_enabled: true, payment_balance_disabled: false, channel_monitor_enabled: true, channel_monitor_mode: 'v1', channel_monitor_show_quota: false, affiliate_enabled: true, model_plaza_enabled: true, available_channels_enabled: true, balance_low_notify_enabled: false, server_timezone: 'Asia/Shanghai', server_utc_offset: '+08:00', sidebar_menu_order: [], sidebar_menu_hidden: [], custom_menu_items: [], version: 'local-preview' } as unknown as PublicSettings
const placeholder = { render: () => h(AppLayout, {}, { default: () => h('section', { class: 'home-panel', style: 'padding:40px;max-width:900px;margin:auto' }, [h('h1', { style: 'font-size:22px' }, '此页面不在本次预览范围'), h('p', { style: 'margin:18px 0' }, '可预览首页、渠道，以及充值、密钥、使用记录和兑换入口。本地不会创建密钥、充值或调用上游。'), h(RouterLink, { to: '/dashboard', class: 'btn btn-primary' }, () => '返回首页')]) }) }
const router = createRouter({ history: createWebHashHistory(), routes: [
  { path: '/', redirect: '/dashboard' }, { path: '/dashboard', name: 'Dashboard', component: DashboardView, meta: { title: '首页', titleKey: 'nav.dashboard' } },
  { path: '/monitor', name: 'ChannelStatus', component: ChannelStatusV1View, meta: { title: '渠道状态', titleKey: 'nav.channelStatus' } },
  { path: '/purchase', name: 'PurchaseSubscription', component: () => import('@/views/user/PaymentView.vue'), meta: { titleKey: 'nav.recharge' } },
  { path: '/keys', name: 'Keys', component: () => import('@/views/user/KeysView.vue'), meta: { titleKey: 'nav.apiKeys' } },
  { path: '/usage', name: 'Usage', component: () => import('@/views/user/UsageView.vue'), meta: { titleKey: 'nav.usage' } },
  { path: '/redeem', name: 'Redeem', component: () => import('@/views/user/RedeemView.vue'), meta: { titleKey: 'nav.redeem' } },
  { path: '/:pathMatch(.*)*', component: placeholder, meta: { title: '本地预览' } },
] })
await loadLocaleMessages('zh'); i18n.global.locale.value = 'zh'
app.use(i18n); app.use(router); await router.isReady(); app.mount('#app')
Object.assign(window, { __previewCalls: mockCalls })
