import crypto from 'node:crypto'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { decodeJwtPayload, openaiIdentityFromOauthTokens } from './openai-jwt.mjs'

const project = fileURLToPath(new URL('../../..', import.meta.url))
const CLIENT_ID = 'app_EMoamEEZ73f0CkXaXp7hrann'
const ACTIVE = new Set(['running', 'awaiting_code', 'committing'])
const STAGES = new Set([
  'starting',
  'web_login',
  'password_accepted',
  'mfa_challenge',
  'mfa_accepted',
  'oauth_authorize',
  'token_exchange',
])
const MESSAGES = {
  invalid_input: '请填写有效邮箱、密码和 2FA 密钥；验证码模式请在提示后输入六位验证码。',
  runtime_unavailable: '账号登录组件暂时不可用。',
  proxy_required: '请选择有效的登录代理。',
  login_busy: '当前管理员已有登录任务，请等待完成或取消。',
  capacity: '登录任务已满，请稍后重试。',
  job_missing: '登录任务已结束或不存在，请重新登录。',
  code_not_expected: '当前任务未等待 2FA 验证码。',
  password_rejected: '账号密码被拒绝，请核对后重试。',
  totp_rejected: '2FA 验证失败，请检查密钥或使用新的验证码。',
  security_check: '上游要求额外安全验证，请改用 OAuth 在官方页面完成验证。',
  preflight_blocked: '登录出口预检被上游拒绝，尚未验证账号密码和 2FA；这不代表密码错误或账号要求额外验证。请检查登录出口，或使用授权链接。',
  access_denied: '登录请求被上游拒绝访问，暂不能确认是否需要额外验证；请使用授权链接，或联系管理员查看失败阶段。',
  email_verification_required: '密码或 2FA 步骤后，上游另要求邮箱验证码；请使用授权链接继续完成邮箱验证。',
  interaction_required: '上游要求邮箱验证码或其他交互，请改用 OAuth 完成登录。',
  phone_verification_required: '上游要求手机号验证，请先在官方页面完成验证。',
  account_deactivated: '上游报告账号已停用或删除。',
  rate_limited: '上游限制了登录频率，请稍后重试。',
  transport_error: '登录连接失败，请检查所选登录出口。',
  proxy_timeout: '所选登录出口连接超时，尚未开始账号或 2FA 验证。请检查代理可用性后重试。',
  runtime_error: '登录组件运行失败，请检查控制端登录组件及依赖。',
  credentials_incomplete: '登录未取得完整 OAuth 凭据。',
  identity_mismatch: '登录结果的账号与输入邮箱不一致，未保存凭据。',
  workspace_mismatch: '登录结果不属于当前账号工作区，未保存凭据。',
  callback_invalid: 'OAuth 回调校验失败，未保存凭据。',
  credential_changed: '登录期间槽位或凭据已改变，请重新登录。',
  commit_failed: '登录成功但凭据保存失败，请重试。',
  protocol_error: '上游登录流程未完成，可重试或改用 OAuth 授权。',
  timeout: '登录超时，请重试。',
  canceled: '登录已取消。',
}

export function loginError(code, status = 400) {
  const error = new Error(MESSAGES[code] || MESSAGES.protocol_error)
  error.code = Object.hasOwn(MESSAGES, code) ? code : 'protocol_error'
  error.status = status
  return error
}

export function normalizePasswordLogin(input = {}) {
  const email = typeof input.email === 'string' ? input.email.trim().toLowerCase() : ''
  const password = typeof input.password === 'string' ? input.password : ''
  let mfaSecret = typeof input.mfa_secret === 'string' ? input.mfa_secret.trim() : ''
  const mfaMode = input.mfa_mode || 'secret'
  if (mfaSecret.startsWith('otpauth://')) {
    try {
      const uri = new URL(mfaSecret)
      if (
        uri.hostname !== 'totp' ||
        (uri.searchParams.get('algorithm') || 'SHA1').toUpperCase() !== 'SHA1' ||
        (uri.searchParams.get('digits') || '6') !== '6' ||
        (uri.searchParams.get('period') || '30') !== '30'
      )
        throw new Error()
      mfaSecret = uri.searchParams.get('secret') || ''
    } catch {
      throw loginError('invalid_input')
    }
  }
  mfaSecret = mfaSecret.replace(/\s|-/g, '').replace(/=+$/, '').toUpperCase()
  if (
    !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) ||
    email.length > 320 ||
    !password.trim() ||
    password.length > 4096 ||
    !['secret', 'code'].includes(mfaMode) ||
    (mfaMode === 'secret' && !/^[A-Z2-7]{16,128}$/.test(mfaSecret))
  )
    throw loginError('invalid_input')
  return { email, password, mfaSecret: mfaMode === 'secret' ? mfaSecret : '' }
}

export function validateLoginCredential(value, email, expectedWorkspace) {
  const credential = {}
  for (const key of ['access_token', 'refresh_token', 'id_token']) {
    if (typeof value?.[key] !== 'string' || !value[key].trim() || value[key].length > 64 * 1024)
      throw loginError('credentials_incomplete')
    credential[key] = value[key]
  }
  const idClaims = decodeJwtPayload(credential.id_token)
  const accessClaims = decodeJwtPayload(credential.access_token)
  const identity = openaiIdentityFromOauthTokens(credential)
  // Tokens originate only from the isolated TLS-verified official login. Claims
  // here bind that result to this request; they are not signature verification.
  if (!idClaims || !identity.email || identity.email.toLowerCase() !== email) throw loginError('identity_mismatch')
  const audience = Array.isArray(idClaims.aud) ? idClaims.aud : [idClaims.aud]
  if (!audience.includes(CLIENT_ID)) throw loginError('credentials_incomplete')
  if (!identity.chatgpt_account_id || (expectedWorkspace && identity.chatgpt_account_id !== expectedWorkspace))
    throw loginError('workspace_mismatch')
  const expires = Number(accessClaims?.exp || 0) * 1000
  if (!expires || expires <= Date.now()) throw loginError('credentials_incomplete')
  return { ...credential, email, chatgpt_account_id: identity.chatgpt_account_id, expires_at: expires }
}

function runtimeEnvironment(root) {
  const env = {}
  // Do not pass panel keys, ambient proxy variables or upstream endpoint overrides.
  for (const key of ['PATH', 'Path', 'SystemRoot', 'WINDIR', 'TEMP', 'TMP', 'HOME', 'LANG', 'TZ']) {
    if (process.env[key]) env[key] = process.env[key]
  }
  env.NODE_ENV = 'production'
  env.KIN_CODEX_LOGIN_RUNTIME = root
  env.TOSUB2_PYTHON =
    process.env.KIN_CODEX_LOGIN_PYTHON ||
    (process.platform === 'win32' ? 'python' : '/opt/vm2api/codex-login-python/bin/python')
  return env
}

export function createPasswordLoginService({
  runtimeRoot = process.env.KIN_CODEX_LOGIN_RUNTIME || path.join(project, 'codex-login-runtime'),
  spawnImpl = spawn,
  timeoutMs = 10 * 60 * 1000,
  retentionMs = 15 * 60 * 1000,
  maxRunning = 4,
  allowDirect = false,
  audit = (event) => process.stdout.write(`${JSON.stringify(event)}\n`),
  available = () =>
    fs.existsSync(path.join(runtimeRoot, 'src/protocol-login.mjs')) &&
    fs.existsSync(path.join(runtimeRoot, 'node_modules/jsdom/package.json')),
} = {}) {
  const jobs = new Map()
  const log = (job, event, metadata = {}) => {
    // Rebuild allowed fields; never spread input, errors, tokens or raw events.
    try { audit({ event: 'password_login_' + event, at: new Date().toISOString(), job_id: job.id, stage: job.stage, status: job.status, reason: job.reason || null, ...metadata }) } catch { /* Diagnostics must not break login. */ }
  }
  const snapshot = (job) =>
    job
      ? {
          id: job.id,
          status: job.status,
          stage: job.stage,
          reason: job.reason || null,
          message: job.reason ? MESSAGES[job.reason] || MESSAGES.protocol_error : null,
          created_at: job.createdAt,
        }
      : null
  const keyOf = (vmId, ownerId) => `${vmId}\0${ownerId}`
  function get(vmId, ownerId) {
    return { available: available(), job: snapshot(jobs.get(keyOf(vmId, ownerId))) }
  }
  function requireJob(vmId, ownerId, id) {
    const job = jobs.get(keyOf(vmId, ownerId))
    if (!job || job.id !== id) throw loginError('job_missing', 404)
    return job
  }
  function stopChild(job) {
    if (!job.child || job.closed) return
    if (!job.child.pid) {
      job.child.kill('SIGKILL')
      return
    }
    if (process.platform !== 'win32') {
      try {
        process.kill(-job.child.pid, 'SIGKILL')
        return
      } catch {
        /* Already exited. */
      }
    } else {
      // Python transport is a child of the Node runner; cancel the owned tree.
      const killer = spawnImpl('taskkill', ['/PID', String(job.child.pid), '/T', '/F'], {
        stdio: 'ignore',
        windowsHide: true,
      })
      killer.once('error', () => job.child.kill('SIGKILL'))
      return
    }
    job.child.kill('SIGKILL')
  }
  function finish(job, status, reason = null) {
    if (!ACTIVE.has(job.status)) return
    clearTimeout(job.timeout)
    job.status = status
    job.reason = reason
    log(job, 'finished')
    job.expiry = setTimeout(() => {
      if (jobs.get(job.key) === job) jobs.delete(job.key)
    }, retentionMs)
    job.expiry.unref?.()
  }
  function cancel(vmId, ownerId, id) {
    const job = requireJob(vmId, ownerId, id)
    if (job.status === 'committing') throw loginError('login_busy', 409)
    if (ACTIVE.has(job.status)) {
      finish(job, 'canceled', 'canceled')
      stopChild(job)
    }
    return snapshot(job)
  }
  function submitCode(vmId, ownerId, id, code) {
    const job = requireJob(vmId, ownerId, id)
    if (job.status !== 'awaiting_code') throw loginError('code_not_expected', 409)
    if (typeof code !== 'string' || !/^\d{6}$/.test(code)) throw loginError('invalid_input')
    job.status = 'running'
    job.child.stdin.write(`${JSON.stringify({ code })}\n`)
    return snapshot(job)
  }
  function start({ vmId, ownerId, input, proxyUrl, expectedWorkspace, commit }) {
    const normalized = normalizePasswordLogin(input)
    if (!available()) throw loginError('runtime_unavailable', 503)
    if (typeof proxyUrl !== 'string' || (!proxyUrl && !allowDirect)) throw loginError('proxy_required')
    if (proxyUrl) {
      let proxy
      try { proxy = new URL(proxyUrl) } catch { throw loginError('proxy_required') }
      if (!['socks5:', 'socks5h:', 'http:', 'https:'].includes(proxy.protocol) || !proxy.hostname)
        throw loginError('proxy_required')
    }
    if ([...jobs.values()].some((job) => job.vmId === vmId && ACTIVE.has(job.status)))
      throw loginError('login_busy', 409)
    if ([...jobs.values()].filter((job) => ACTIVE.has(job.status)).length >= maxRunning)
      throw loginError('capacity', 429)
    const key = keyOf(vmId, ownerId)
    clearTimeout(jobs.get(key)?.expiry)
    const job = {
      key,
      vmId,
      id: crypto.randomUUID(),
      status: 'running',
      stage: 'starting',
      createdAt: Date.now(),
      closed: false,
    }
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'vm2api-login-'))
    fs.chmodSync(directory, 0o700)
    jobs.set(key, job)
    log(job, 'started')
    let buffer = '',
      credential = null
    const fail = (reason) => {
      finish(job, 'failed', reason)
      stopChild(job)
    }
    const cleanup = () => {
      try {
        fs.rmSync(directory, { recursive: true, force: true, maxRetries: 3, retryDelay: 50 })
      } catch {
        /* Process teardown may still hold a file on Windows. */
      }
    }
    try {
      job.child = spawnImpl(process.execPath, [path.join(project, 'scripts/codex-login/runner.mjs')], {
        cwd: directory,
        env: runtimeEnvironment(runtimeRoot),
        stdio: ['pipe', 'pipe', 'ignore'],
        detached: process.platform !== 'win32',
        windowsHide: true,
      })
      job.child.stdin.on('error', () => fail('runtime_error'))
      job.child.stdout.on('data', (chunk) => {
        if (!ACTIVE.has(job.status)) return
        buffer += chunk.toString('utf8')
        if (Buffer.byteLength(buffer) > 256 * 1024) return fail('protocol_error')
        let split
        while ((split = buffer.indexOf('\n')) >= 0) {
          const line = buffer.slice(0, split)
          buffer = buffer.slice(split + 1)
          try {
            const event = JSON.parse(line)
            if (event.type === 'stage' && STAGES.has(event.stage)) { job.stage = event.stage; log(job, 'stage') }
            else if (event.type === 'diagnostic') {
              const target = ['other', 'chatgpt', 'auth', 'token', 'preflight'].includes(event.target) ? event.target : 'other'
              const page = ['login', 'password', 'mfa', 'mfa_verify', 'email_otp_send', 'email_otp', 'add_phone', 'consent', 'workspace'].includes(event.page) ? event.page : 'unknown'
              const http_status = Number.isInteger(event.status) && event.status >= 100 && event.status <= 599 ? event.status : 0
              log(job, 'response', { target, page, http_status, challenge: event.challenge === true })
            }
            else if (event.type === 'code_required' && job.status === 'running') job.status = 'awaiting_code'
            else if (event.type === 'credential' && job.status === 'running' && !credential)
              credential = validateLoginCredential(event.credential, normalized.email, expectedWorkspace)
            else if (event.type === 'error')
              fail(Object.hasOwn(MESSAGES, event.reason) ? event.reason : 'protocol_error')
            else fail('protocol_error')
          } catch (error) {
            fail(Object.hasOwn(MESSAGES, error?.code) ? error.code : 'protocol_error')
          }
          if (!ACTIVE.has(job.status)) break
        }
      })
      job.child.once('error', () => {
        fail('runtime_error')
        cleanup()
      })
      job.child.once('close', async (code) => {
        job.closed = true
        try {
          if (!ACTIVE.has(job.status)) return
          if (code !== 0 || !credential || buffer.trim()) return fail('protocol_error')
          clearTimeout(job.timeout)
          job.status = 'committing'
          await commit(credential)
          finish(job, 'succeeded')
        } catch (error) {
          finish(job, 'failed', error?.code === 'credential_changed' ? 'credential_changed' : 'commit_failed')
        } finally {
          credential = null
          cleanup()
        }
      })
      job.timeout = setTimeout(() => fail('timeout'), timeoutMs)
      job.timeout.unref?.()
      job.child.stdin.write(`${JSON.stringify({ ...normalized, proxyUrl, expectedWorkspace })}\n`)
      normalized.password = ''
      normalized.mfaSecret = ''
    } catch {
      fail('runtime_error')
      cleanup()
    }
    return snapshot(job)
  }
  return { get, start, cancel, submitCode }
}
