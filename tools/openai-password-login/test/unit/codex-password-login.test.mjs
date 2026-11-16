import test from 'node:test'
import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { PassThrough } from 'node:stream'
import fs from 'node:fs'
import {
  createPasswordLoginService,
  normalizePasswordLogin,
  validateLoginCredential,
} from '../../src/lib/oauth/codex-password-login.mjs'
import { validateCallback, observeAuthResponse } from '../../scripts/codex-login/hooks.mjs'

const email = 'test@example.com'
const input = { email, password: ' synthetic password ', mfa_mode: 'secret', mfa_secret: 'JBSWY3DPEHPK3PXP' }
const jwt = (body) => `test.${Buffer.from(JSON.stringify(body)).toString('base64url')}.signature`
function tokens(overrides = {}) {
  return {
    access_token: jwt({
      exp: Math.floor(Date.now() / 1000) + 3600,
      'https://api.openai.com/auth': { chatgpt_account_id: 'workspace-1' },
    }),
    refresh_token: 'synthetic-refresh',
    id_token: jwt({
      email,
      aud: 'app_EMoamEEZ73f0CkXaXp7hrann',
      'https://api.openai.com/auth': { chatgpt_account_id: 'workspace-1' },
    }),
    ...overrides,
  }
}
const tick = () => new Promise((resolve) => setImmediate(resolve))
function fixture(options = {}) {
  const children = []
  const committed = []
  const spawnImpl = (command, args, config) => {
    const child = new EventEmitter()
    child.stdin = new PassThrough()
    child.stdout = new PassThrough()
    child.input = ''
    child.stdin.on('data', (data) => {
      child.input += data
    })
    child.kill = () => {
      child.emit('close', 1)
      return true
    }
    child.event = (data) => child.stdout.write(JSON.stringify(data) + '\n')
    child.config = config
    child.command = command
    child.args = args
    children.push(child)
    return child
  }
  const service = createPasswordLoginService({ available: () => true, spawnImpl, audit: () => {}, ...options })
  const start = (extra = {}) =>
    service.start({
      vmId: 'vm-test',
      ownerId: 'alice',
      input,
      proxyUrl: 'socks5h://proxy.test:1080',
      commit: async (credential) => committed.push(credential),
      ...extra,
    })
  return { service, start, children, committed }
}

test('normal SUB login explicitly allows direct egress without changing VM proxy defaults', () => {
  const direct = fixture({ allowDirect: true })
  direct.start({ proxyUrl: '' })
  assert.equal(direct.children.length, 1)
  assert.equal(JSON.parse(direct.children[0].input.trim()).proxyUrl, '')
  direct.service.cancel('vm-test', 'alice', direct.service.get('vm-test', 'alice').job.id)
  const vm = fixture()
  assert.throws(() => vm.start({ proxyUrl: '' }), { code: 'proxy_required' })
})

test('password input preserves password, normalizes Base32/URI and supports deferred code', () => {
  assert.equal(normalizePasswordLogin(input).password, input.password)
  assert.equal(normalizePasswordLogin({ ...input, mfa_secret: 'jbsw y3dp-ehpk3pxp=' }).mfaSecret, input.mfa_secret)
  assert.equal(
    normalizePasswordLogin({ ...input, mfa_secret: 'otpauth://totp/test?secret=JBSWY3DPEHPK3PXP' }).mfaSecret,
    input.mfa_secret,
  )
  assert.equal(normalizePasswordLogin({ ...input, mfa_mode: 'code', mfa_secret: '' }).mfaSecret, '')
  for (const patch of [
    { email: 'bad' },
    { password: '' },
    { mfa_secret: '123456' },
    { mfa_mode: 'unknown' },
    { mfa_secret: 'otpauth://totp/test?secret=JBSWY3DPEHPK3PXP&algorithm=SHA256' },
  ]) {
    assert.throws(() => normalizePasswordLogin({ ...input, ...patch }), { code: 'invalid_input' })
  }
})

test('credential validation binds identity/client/workspace/expiry and removes echoed secrets', () => {
  const result = validateLoginCredential(
    { ...tokens(), password: 'do-not-return', arbitrary: 'ignored' },
    email,
    'workspace-1',
  )
  assert.equal(result.email, email)
  assert.equal(result.password, undefined)
  assert.equal(result.arbitrary, undefined)
  assert.throws(() => validateLoginCredential(tokens(), 'other@example.com'), { code: 'identity_mismatch' })
  assert.throws(() => validateLoginCredential(tokens(), email, 'other'), { code: 'workspace_mismatch' })
  assert.throws(() => validateLoginCredential(tokens({ id_token: jwt({ email, aud: 'wrong' }) }), email), {
    code: 'credentials_incomplete',
  })
  assert.throws(() => validateLoginCredential(tokens({ access_token: jwt({ exp: 1 }) }), email), {
    code: 'credentials_incomplete',
  })
})

test('login uses private stdio, waits for code, commits only complete result and hides secrets', async () => {
  const { service, start, children, committed } = fixture()
  const job = start({ input: { ...input, mfa_mode: 'code', mfa_secret: '' } })
  const child = children[0]
  assert.match(child.args[0], /scripts[\\/]codex-login[\\/]runner.mjs$/)
  assert.equal(child.config.env.CHATGPT_LOGIN_PASSWORD, undefined)
  assert.equal(child.config.env.HTTPS_PROXY, undefined)
  assert.equal(child.config.env.KIN_CODEX_LOGIN_RUNTIME.endsWith('codex-login-runtime'), true)
  assert.equal(JSON.parse(child.input).proxyUrl, 'socks5h://proxy.test:1080')
  assert.throws(() => start({ ownerId: 'bob' }), { code: 'login_busy' })
  child.event({ type: 'stage', stage: 'mfa_challenge' })
  child.event({ type: 'code_required' })
  assert.equal(service.get('vm-test', 'alice').job.status, 'awaiting_code')
  assert.equal(service.get('vm-test', 'bob').job, null)
  assert.throws(() => service.submitCode('vm-test', 'bob', job.id, '123456'), { code: 'job_missing' })
  service.submitCode('vm-test', 'alice', job.id, '123456')
  assert.match(child.input, /"code":"123456"/)
  child.event({ type: 'credential', credential: tokens() })
  assert.equal(committed.length, 0)
  child.emit('close', 0)
  await tick()
  assert.equal(committed.length, 1)
  const view = service.get('vm-test', 'alice')
  assert.equal(view.job.status, 'succeeded')
  for (const secret of ['synthetic', '123456', 'access_token', 'refresh_token', 'proxy.test'])
    assert.equal(JSON.stringify(view).includes(secret), false)
  assert.equal(fs.existsSync(child.config.cwd), false)
})

test('cancel and process failures never commit late results', async () => {
  for (const failure of ['cancel', 'error', 'invalid-result', 'exit']) {
    const { service, start, children, committed } = fixture()
    const job = start()
    const child = children[0]
    if (failure === 'cancel') service.cancel('vm-test', 'alice', job.id)
    if (failure === 'error') child.event({ type: 'error', reason: 'raw password synthetic' })
    if (failure === 'invalid-result') child.event({ type: 'credential', credential: { access_token: 'bad' } })
    if (failure === 'exit') child.emit('close', 1)
    child.event({ type: 'credential', credential: tokens() })
    child.emit('close', 0)
    await tick()
    assert.equal(committed.length, 0)
    assert.ok(['canceled', 'failed'].includes(service.get('vm-test', 'alice').job.status))
    assert.equal(JSON.stringify(service.get('vm-test', 'alice')).includes('synthetic'), false)
  }
})

test('timeout stops owned task and failed commit reports a bounded error', async () => {
  const one = fixture({ timeoutMs: 10 })
  one.start()
  await new Promise((resolve) => setTimeout(resolve, 25))
  assert.equal(one.service.get('vm-test', 'alice').job.reason, 'timeout')
  assert.equal(one.committed.length, 0)
  const two = fixture()
  two.start({
    commit: async () => {
      throw new Error('secret external body')
    },
  })
  two.children[0].event({ type: 'credential', credential: tokens() })
  two.children[0].emit('close', 0)
  await tick()
  assert.equal(two.service.get('vm-test', 'alice').job.reason, 'commit_failed')
})

test('runtime, proxy and capacity validation prevent launching tasks', () => {
  assert.throws(() => fixture({ available: () => false }).start(), { code: 'runtime_unavailable' })
  const { start, service } = fixture({ maxRunning: 1 })
  assert.throws(() => start({ proxyUrl: '' }), { code: 'proxy_required' })
  const job = start()
  assert.throws(() => start({ vmId: 'another' }), { code: 'capacity' })
  service.cancel('vm-test', 'alice', job.id)
})

test('OAuth callbacks require exact redirect, unique code/state and PKCE', () => {
  const redirect = 'http://localhost:1455/auth/callback'
  const valid = { callbackUrl: `${redirect}?code=one&state=bound`, state: 'bound', codeVerifier: 'a'.repeat(43) }
  assert.doesNotThrow(() => validateCallback(valid, redirect))
  for (const suffix of ['?code=one&state=other', '?code=one&code=two&state=bound', '?code=one&state=bound#other']) {
    assert.throws(() => validateCallback({ ...valid, callbackUrl: redirect + suffix }, redirect), /callback_invalid/)
  }
  assert.throws(
    () => validateCallback({ ...valid, callbackUrl: 'https://other.test/?code=one&state=bound' }, redirect),
    /callback_invalid/,
  )
  assert.throws(
    () => observeAuthResponse({ res: { status: 200 }, text: JSON.stringify({ page: { type: 'add_phone' } }) }),
    /phone_verification_required/,
  )
})

test('proxy preflight failures preserve actionable reasons without leaking proxy secrets', async () => {
  const { failureReason } = await import('../../scripts/codex-login/hooks.mjs')
  const cases = [
    ['代理检测失败：Timeout: Failed to perform, curl: (28) Connection timed out after 30002 milliseconds.', 'proxy_timeout'],
    ['代理检测失败：HTTP 403，返回安全校验页面', 'preflight_blocked'],
    ['代理检测失败：HTTP 429', 'rate_limited'],
    ['代理检测失败：HTTP 407 proxy password=synthetic', 'transport_error'],
  ]
  for (const [message, reason] of cases) {
    assert.equal(failureReason(new Error(message)), reason)
    const f = fixture()
    f.start()
    f.children[0].event({ type: 'error', reason })
    const status = f.service.get('vm-test', 'alice')
    assert.equal(status.job.reason, reason)
    assert.equal(JSON.stringify(status).includes('synthetic'), false)
    assert.equal(f.committed.length, 0)
  }
})

test('diagnostics keep fixed metadata and distinguish preflight, challenge and email OTP', async () => {
  const { configureLogin, reportLoginResponse, failureReason } = await import('../../scripts/codex-login/hooks.mjs')
  const diagnostics = []
  configureLogin({}, { diagnostic: d => diagnostics.push(d) })
  reportLoginResponse({ status: 403, headers: { 'content-type': 'text/html', 'set-cookie': 'SECRET-cookie' }, body: '<html>Forbidden SECRET-body</html>', url: 'https://auth.openai.com/path?code=SECRET-code' })
  assert.equal(failureReason(new Error('PROXY_RISK_CONTROL: private')), 'access_denied')
  reportLoginResponse({ status: 403, headers: { 'cf-mitigated': 'challenge' }, body: 'SECRET-body', url: 'https://auth.openai.com/' })
  assert.equal(failureReason(new Error('PROXY_RISK_CONTROL: private')), 'security_check')
  assert.equal(failureReason(new Error('代理检测失败：HTTP 403 SECRET')), 'preflight_blocked')
  assert.throws(() => observeAuthResponse({ res: { status: 200 }, text: JSON.stringify({ page: { type: 'email_otp_send' } }) }), /email_verification_required/)
  const logs = []
  const f = fixture({ audit: event => logs.push(event) })
  f.start()
  f.children[0].event({ type: 'diagnostic', target: 'auth', status: 403, page: 'SECRET-page', challenge: false, password: 'SECRET', url: 'SECRET' })
  f.children[0].event({ type: 'error', reason: 'preflight_blocked' })
  assert.equal(logs.at(-1).stage, 'starting')
  assert.equal(logs.at(-1).reason, 'preflight_blocked')
  assert.equal(logs.find(l => l.http_status === 403).page, 'unknown')
  assert.equal(JSON.stringify([diagnostics, logs]).includes('SECRET'), false)
})

test('official combined username/password rejection produces a bounded credential error', async () => {
  const { failureReason } = await import('../../scripts/codex-login/hooks.mjs')
  const { service, start, children } = fixture()
  start()
  let failure
  try {
    observeAuthResponse({
      res: { status: 401 },
      text: JSON.stringify({ error: { code: 'invalid_username_or_password', message: 'private upstream details' } }),
    })
  } catch (error) {
    failure = error
  }
  assert.ok(failure)
  children[0].event({ type: 'error', reason: failureReason(failure) })
  const view = service.get('vm-test', 'alice')
  assert.equal(view.job.reason, 'password_rejected')
  assert.match(view.job.message, /密码/)
  assert.equal(JSON.stringify(view).includes('private upstream details'), false)
})
