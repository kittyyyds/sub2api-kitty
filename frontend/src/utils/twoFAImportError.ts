// Only known local preparation errors are shown. Never echo upstream text,
// request bodies, credentials or arbitrary exception messages into the importer.
const preparationMessages = new Map<string, string>([
  ['没有可分配的 IP：须出口已验证、当前空闲、历史未满且未被预留；未切换直连',
    '自动分配 IP 失败：没有符合条件的空闲代理。请在 IP 管理添加并启用当前未被账号使用、历史未达上限的代理，再点击继续。尚未开始账号登录，未验证密码或 2FA。'],
  ['所选 IP 已达历史上限、被预留或不可用',
    '所选 IP 已达历史上限、被其他导入任务预留或不可用。请检查 IP 管理；账号登录尚未开始。'],
  ['所选代理不可用；未创建直连登录任务',
    '所选代理不可用，账号登录尚未开始。请检查代理设置；系统没有改用服务器直连。'],
  ['代理出口探测不可用，未分配代理',
    '代理出口检测服务暂不可用，无法分配 IP；账号登录尚未开始。'],
  ['请先在凭证运营中启用凭据加密，再进行 2FA 导入',
    '请先在凭证运营中启用凭据加密，再进行 2FA 导入。'],
  ['请在凭证守护中配置有效的重登接口',
    '尚未配置可用的首次登录服务，请检查重登服务配置。'],
  ['登录任务过多，请稍后重试', '后台登录任务已满，请稍后继续。'],
])

export function twoFAStartFailureMessage(cause: unknown): string | undefined {
  if (!cause || typeof cause !== 'object') return undefined
  const error = cause as { message?: unknown; response?: { data?: { message?: unknown } } }
  const message = error.response?.data?.message ?? error.message
  return typeof message === 'string' ? preparationMessages.get(message) : undefined
}
