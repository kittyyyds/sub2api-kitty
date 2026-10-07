import { AxiosError, type AxiosAdapter } from 'axios'
import type { UserDashboardStats } from '@/api/usage'
import type { UserMonitorView } from '@/api/channelMonitor'
import type { UserAnnouncement, User } from '@/types'

export const params = new URLSearchParams(location.search)
export const scenario = params.get('scenario') || 'normal'
export const sampleUser = { id: 900001, username: 'Kitty', email: 'preview@example.invalid', role: 'user', status: 'active', balance: 128.56, concurrency: 5, created_at: '2026-09-01T00:00:00Z', updated_at: '2026-10-07T00:00:00Z' } as User
export const sampleStats: UserDashboardStats = {
  total_api_keys: 4, active_api_keys: 3, total_requests: 12864, total_input_tokens: 42000000, total_output_tokens: 5800000, total_cache_creation_tokens: 7200000, total_cache_read_tokens: 63400000,
  total_tokens: 118400000, total_cost: 1245.42, total_actual_cost: 246.8326, today_requests: 386, today_input_tokens: 860000, today_output_tokens: 120000, today_cache_creation_tokens: 200000, today_cache_read_tokens: 1100000,
  today_tokens: 2280000, today_cost: 42.60, today_actual_cost: 8.5214, average_duration_ms: 1240, rpm: 12.8, tpm: 86500,
  by_platform: [{ platform: 'openai', total_requests: 9540, total_tokens: 94000000, total_actual_cost: 188.26, today_requests: 290, today_tokens: 1800000, today_actual_cost: 6.30 }, { platform: 'anthropic', total_requests: 3324, total_tokens: 24400000, total_actual_cost: 58.5726, today_requests: 96, today_tokens: 480000, today_actual_cost: 2.2214 }],
}
export const announcements: UserAnnouncement[] = Array.from({ length: 27 }, (_, i) => ({
  id: 100 - i, title: ['欢迎来到 Kitty，开始你的创作之旅', 'API 接入指南：从第一条请求开始', '关于近期渠道状态与模型更新', '账户余额与用量统计说明', '让你的 API 密钥更安全', '服务更新与维护记录'][i % 6] + (i >= 6 ? ` · ${i + 1}` : ''),
  content: i === 0 ? '这里是你的新首页。\n\n**把时间留给创造，把连接交给 Kitty。**\n\n我们把账户用量、余额和服务动态放在一起，让每次打开控制台，都能快速找到你关心的信息。\n\n> 需要了解模型当前的可用情况？打开「渠道状态」，即可查看实时探测结果和历史可用性。\n\n### 开始之前\n\n- 在「API 密钥」中创建用于应用的密钥。\n- 按需选择分组与模型，妥善保管密钥。\n- 在「用量分析」中查看每次调用的实际消费。\n\n感谢你的信任，祝你今天也有新的灵感。' : '这是用于本地预览的示例公告。\n\n公告支持 **Markdown**、列表、链接和完整历史内容。正式运行时，首页只显示当前用户有权查看且仍在发布期内的公告。\n\n阅读后可点击「标记已读」，状态与顶部公告中心同步。',
  notify_mode: 'silent', created_at: new Date(Date.UTC(2026, 9, 7 - i, 1)).toISOString(), updated_at: new Date(Date.UTC(2026, 9, 7 - i, 1)).toISOString(), read_at: i < 3 ? undefined : '2026-10-06T01:00:00Z',
}))
const channelRows = [
  ['Codex · 优选', 'openai', 'gpt-6-astra', .2], ['Codex · 福利', 'openai', 'gpt-6-astra', .12], ['GPT · 标准', 'openai', 'gpt-5.6-sol', .4], ['GPT · 高速', 'openai', 'gpt-5.6-terra', .5],
  ['Claude · 优选', 'anthropic', 'claude-opus-4-6', .8], ['Claude · 标准', 'anthropic', 'claude-sonnet-4-6', .6], ['Claude · 轻量', 'anthropic', 'claude-haiku-4-5', .4], ['Grok · 标准', 'grok', 'grok-4', .3],
  ['Gemini · 高速', 'gemini', 'gemini-2.5-pro', .4], ['DeepSeek · 专线', 'deepseek', 'deepseek-chat', .2], ['Kimi · 专线', 'kimi', 'kimi-k2.5', .3], ['GLM · 专线', 'zhipu', 'glm-4.7', .3],
] as const
export const monitors: UserMonitorView[] = channelRows.map(([name, provider, model], i) => ({
  id: i + 1, name, provider, group_name: name, primary_model: model, primary_status: i === 5 ? 'degraded' : 'operational', primary_latency_ms: 460 + i * 137,
  primary_ping_latency_ms: 100, availability_7d: i === 5 ? 97.42 : 99.98 - i * .013, extra_models: [],
  timeline: Array.from({ length: 60 }, (_, j) => ({ status: i === 5 && j % 8 === 0 ? 'degraded' : (i === 8 && j === 25 ? 'failed' : 'operational'), latency_ms: 600 + (j % 7) * 55, ping_latency_ms: 100, checked_at: new Date(Date.now() - j * 60000).toISOString() })),
}))
export const mockCalls: string[] = []
export const fixtureAdapter: AxiosAdapter = async config => {
  const path = (config.url || '').split('?')[0]
  mockCalls.push(`${config.method} ${path}`)
  await new Promise(resolve => setTimeout(resolve, 100))
  if (config.method !== 'get' && !/^\/announcements\/\d+\/read$/.test(path)) {
    const response = { data: { code: 400, message: '本地预览不提交订单、不创建密钥、不兑换余额。' }, status: 400, statusText: 'Preview only', headers: {}, config }
    throw new AxiosError(response.data.message, 'ERR_BAD_REQUEST', config, undefined, response)
  }
  if (scenario === 'error' && (path.includes('/usage/dashboard/stats') || path === '/announcements')) throw new AxiosError('本地模拟加载失败', 'ERR_NETWORK', config)
  let data: unknown
  if (path === '/auth/me' || path === '/user/profile') data = { ...sampleUser, run_mode: scenario === 'simple' ? 'simple' : 'standard' }
  else if (path === '/usage/dashboard/stats') data = sampleStats
  else if (path === '/announcements') data = scenario === 'empty' ? [] : announcements
  else if (/^\/announcements\/\d+\/read$/.test(path)) {
    if (scenario === 'read-error') throw new AxiosError('本地模拟保存失败', 'ERR_NETWORK', config)
    const row = announcements.find(a => a.id === Number(path.split('/')[2])); if (row) row.read_at = new Date().toISOString()
    data = { message: 'ok' }
  }
  else if (path === '/channel-monitors') data = { items: scenario === 'empty' ? [] : monitors }
  else if (/^\/channel-monitors\/\d+\/status$/.test(path)) {
    const row = monitors.find(m => m.id === Number(path.split('/')[2]))!
    data = { id: row.id, name: row.name, provider: row.provider, group_name: row.group_name, models: [{ model: row.primary_model, latest_status: row.primary_status, latest_latency_ms: row.primary_latency_ms, availability_7d: row.availability_7d, availability_15d: row.availability_7d - .2, availability_30d: row.availability_7d - .4, avg_latency_7d_ms: row.primary_latency_ms }] }
  }
  else if (path === '/groups/available') data = channelRows.map(([name, platform, , rate_multiplier], i) => ({ id: i + 1, name, platform, rate_multiplier, status: 'active' }))
  else if (path === '/groups/rates') data = { 2: .10 }
  else if (path === '/user/platform-quotas') data = { platform_quotas: [] }
  else if (path === '/usage/dashboard/trend') data = { trend: Array.from({ length: 7 }, (_, i) => ({ date: `2026-10-0${i + 1}`, requests: 170 + i * 36, total_tokens: 1080000 + i * 200000, input_tokens: 700000 + i * 55000, output_tokens: 90000 + i * 5000, cache_read_tokens: 200000, cache_creation_tokens: 90000, cost: 25 + i * 5, actual_cost: 5 + i * 1.1 })) }
  else if (path === '/usage/dashboard/models') data = { models: [{ model: 'gpt-6-astra', requests: 2100, total_tokens: 10600000, actual_cost: 24.5, cost: 122.5 }, { model: 'claude-sonnet-4-6', requests: 980, total_tokens: 3800000, actual_cost: 12.8, cost: 64 }] }
  else if (path === '/usage') data = { items: [], total: 0, page: 1, page_size: 20, pages: 1 }
  else if (path === '/usage/stats') data = { total_requests: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0, endpoints: [] }
  else if (path === '/usage/dashboard/snapshot-v2') data = { trend: [], groups: [], models: [] }
  else if (path === '/redeem/history') data = { items: [], total: 0, page: 1, page_size: 20, pages: 1 }
  else if (path === '/payment/checkout-info') data = {
    methods: { alipay: { currency: 'CNY', daily_limit: 0, daily_used: 0, daily_remaining: 0, single_min: 1, single_max: 5000, fee_rate: 0, available: true } },
    global_min: 1, global_max: 5000, plans: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 0, recharge_fee_rate: 0,
    help_text: '', help_image_url: '', stripe_publishable_key: '', recharge_bonus_rules: [], recharge_bonus_summary: { available_amount: 0, active_grants: 0 }, recharge_bonus_tiers: [], recharge_bonus_mode: 'bonus', recharge_bonus_notice: '',
  }
  else if (path === '/keys') data = { items: [], total: 0, pages: 1 }
  else if (path.includes('subscriptions')) data = []
  else if (path === '/settings/public') data = (window as Window & { __APP_CONFIG__?: unknown }).__APP_CONFIG__
  else if (config.method === 'get') data = {}
  else throw new Error('本地预览不执行此写入操作：' + path)
  return { data: { code: 0, data: structuredClone(data) }, status: 200, statusText: 'OK', headers: {}, config }
}
