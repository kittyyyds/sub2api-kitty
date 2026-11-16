import test from 'node:test'
import assert from 'node:assert/strict'
import { createLoginServer } from '../server.mjs'

const secret = 'synthetic-internal-service-secret-for-test'
const id = '00112233-4455-4677-8899-aabbccddeeff'
async function fixture(t, ttl = 5000) {
  const jobs = new Map(), commits = new Map(), starts = []
  const key = (vm, owner) => `${vm}:${owner}`
  const service = {
    get: (vm, owner) => ({ available: true, job: jobs.get(key(vm, owner)) }),
    start: (input) => {
      const job = { id, status: 'running', stage: 'starting' }
      jobs.set(key(input.vmId, input.ownerId), job)
      commits.set(input.ownerId, input.commit)
      starts.push({ proxy: input.proxyUrl, owner: input.ownerId, email: input.input.email })
      return job
    },
    cancel: (vm, owner) => { const job = jobs.get(key(vm, owner)); job.status = 'canceled'; return job },
    submitCode: (vm, owner) => { const job = jobs.get(key(vm, owner)); job.status = 'running'; return job },
  }
  const server = createLoginServer({ secret, service, resultTTL: ttl })
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  t.after(() => new Promise((resolve) => { server.closeAllConnections(); server.close(resolve) }))
  const url = `http://127.0.0.1:${server.address().port}`
  const call = async (body, auth = true) => {
    const res = await fetch(`${url}/v1/login`, { method: 'POST', headers: { ...(auth ? { Authorization: `Bearer ${secret}` } : {}), 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    return { status: res.status, body: await res.json() }
  }
  const succeed = async (owner) => {
    await commits.get(owner)({ access_token: 'synthetic-access', refresh_token: 'synthetic-refresh', id_token: 'synthetic-id' })
    jobs.get(key(`sub-oauth-${owner}`, owner)).status = 'succeeded'
  }
  return { call, succeed, starts }
}

test('internal authentication and owner binding protect one-time OAuth results', async (t) => {
  const f = await fixture(t)
  const input = { email: 'owner@example.test', password: 'synthetic-password', mfa_secret: 'JBSWY3DPEHPK3PXP' }
  assert.equal((await f.call({ action: 'start', owner_id: 1, input }, false)).status, 401)
  const started = await f.call({ action: 'start', owner_id: 1, input, proxy_id: 7, proxy_url: 'socks5h://proxy.test:1080' })
  assert.equal(started.status, 202)
  assert(!JSON.stringify(started).includes(input.password))
  assert.equal((await f.call({ action: 'status', owner_id: 2, job_id: id })).status, 404)
  assert.equal((await f.call({ action: 'start', owner_id: 1, input })).status, 409)
  await f.succeed('1')
  assert.equal((await f.call({ action: 'result', owner_id: 2, job_id: id })).status, 404)
  const result = await f.call({ action: 'result', owner_id: 1, job_id: id })
  assert.equal(result.body.proxy_id, 7)
  assert.equal(result.body.credential.refresh_token, 'synthetic-refresh')
  assert.equal((await f.call({ action: 'result', owner_id: 1, job_id: id })).status, 404)
})

test('explicit direct egress is supported and expired results disappear', async (t) => {
  const f = await fixture(t, 15)
  await f.call({ action: 'start', owner_id: 3, input: { email: 'test@example.test' } })
  assert.equal(f.starts[0].proxy, '')
  await f.succeed('3')
  await new Promise((resolve) => setTimeout(resolve, 30))
  assert.equal((await f.call({ action: 'result', owner_id: 3, job_id: id })).status, 404)
})

test('canceling a completed task also destroys any unclaimed grant', async (t) => {
  const f = await fixture(t)
  await f.call({ action: 'start', owner_id: 4, input: {} })
  await f.succeed('4')
  assert.equal((await f.call({ action: 'cancel', owner_id: 4, job_id: id })).status, 200)
  assert.equal((await f.call({ action: 'result', owner_id: 4, job_id: id })).status, 409)
})
