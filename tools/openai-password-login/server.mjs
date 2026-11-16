import crypto from 'node:crypto'
import fs from 'node:fs'
import http from 'node:http'
import { pathToFileURL } from 'node:url'
import { createPasswordLoginService, loginError } from './src/lib/oauth/codex-password-login.mjs'

// This service only obtains Codex OAuth grants. It does not create VM slots,
// forward inference requests, store passwords, or expose any Docker operations.
export function createLoginServer({ secret, service = createPasswordLoginService({ allowDirect: true }), resultTTL = 5 * 60_000 } = {}) {
  if (typeof secret !== 'string' || secret.length < 32) throw new Error('Login service secret is missing')
  const expected = crypto.createHash('sha256').update(`Bearer ${secret}`).digest()
  const results = new Map()
  const timers = new Map()
  const ownerKey = (owner) => `sub-oauth-${owner}`
  function forget(owner) {
    clearTimeout(timers.get(owner))
    timers.delete(owner)
    results.delete(owner)
  }
  function respond(res, status, value) {
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
    res.end(JSON.stringify(value))
  }
  const server = http.createServer(async (req, res) => {
    const received = crypto.createHash('sha256').update(req.headers.authorization || '').digest()
    if (!crypto.timingSafeEqual(received, expected)) return respond(res, 401, { code: 'unauthorized' })
    try {
      if (req.method === 'GET' && req.url === '/health') {
        return respond(res, 200, { available: service.get('health', 'health').available })
      }
      if (req.method !== 'POST' || req.url !== '/v1/login') return respond(res, 404, { code: 'not_found' })
      let bytes = 0
      const chunks = []
      for await (const chunk of req) {
        bytes += chunk.length
        if (bytes > 64 * 1024) return respond(res, 413, { code: 'invalid_input' })
        chunks.push(chunk)
      }
      let body
      try { body = JSON.parse(Buffer.concat(chunks).toString('utf8')) } catch { throw loginError('invalid_input') }
      const owner = String(body.owner_id || '')
      if (!/^[1-9][0-9]{0,18}$/.test(owner)) throw loginError('invalid_input')
      const vm = ownerKey(owner)
      if (body.action === 'start') {
        const current = service.get(vm, owner).job
        if (current && ['running', 'awaiting_code', 'committing'].includes(current.status)) throw loginError('login_busy', 409)
        if (body.proxy_id != null && (!Number.isSafeInteger(body.proxy_id) || body.proxy_id <= 0)) throw loginError('invalid_input')
        let jobID
        const job = service.start({
          vmId: vm, ownerId: owner, input: body.input, proxyUrl: body.proxy_url || '',
          commit: async (credential) => {
            forget(owner)
            results.set(owner, { id: jobID, credential, proxy_id: body.proxy_id ?? null })
            const timer = setTimeout(() => forget(owner), resultTTL)
            timer.unref()
            timers.set(owner, timer)
          },
        })
        jobID = job.id
        forget(owner)
        // Remove secret-bearing request objects after the child received stdin.
        body.input = null
        body.proxy_url = ''
        return respond(res, 202, job)
      }
      const job = service.get(vm, owner).job
      if (!job || job.id !== body.job_id) throw loginError('job_missing', 404)
      if (body.action === 'status') return respond(res, 200, job)
      if (body.action === 'code') return respond(res, 200, service.submitCode(vm, owner, job.id, body.code))
      if (body.action === 'cancel') {
        const result = service.cancel(vm, owner, job.id)
        forget(owner)
        return respond(res, 200, result)
      }
      if (body.action === 'result') {
        if (job.status !== 'succeeded') throw loginError('login_busy', 409)
        const result = results.get(owner)
        if (!result || result.id !== job.id) throw loginError('job_missing', 404)
        forget(owner)
        return respond(res, 200, result)
      }
      throw loginError('invalid_input')
    } catch (error) {
      // Only fixed error codes/messages from the owned adapter cross this boundary.
      const safe = loginError(error?.code, error?.status || 400)
      respond(res, safe.status, { code: safe.code, message: safe.message })
    }
  })
  server.requestTimeout = 15_000
  server.headersTimeout = 10_000
  server.maxHeadersCount = 32
  server.on('close', () => { for (const owner of timers.keys()) forget(owner) })
  return server
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const secret = fs.readFileSync(process.env.OPENAI_PASSWORD_LOGIN_SECRET_FILE, 'utf8').trim()
  const server = createLoginServer({ secret })
  server.listen(8098, '0.0.0.0')
  process.on('SIGTERM', () => { server.close(); setTimeout(() => process.exit(0), 1000).unref() })
}
