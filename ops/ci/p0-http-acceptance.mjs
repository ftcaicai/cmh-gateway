import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import http from 'node:http'

// Only targets a local isolated fixture. It creates one synthetic root using
// official setup/login, never modifies production or displays bearer/cookies.
const base = process.env.CMH_ACCEPTANCE_BASE || 'http://127.0.0.1:38080'
assert.equal(new URL(base).hostname, '127.0.0.1', 'acceptance must target loopback')
let cookie = ''
async function request(path, { method = 'GET', body, token, expectedSID, sendCookie = true, headers = {} } = {}) {
  if (headers.Host) {
    return await new Promise((resolve, reject) => {
      const req = http.request(new URL(base + path), { method, headers }, res => {
        const chunks = []
        res.on('data', chunk => chunks.push(chunk))
        res.on('end', () => resolve({ status: res.statusCode, text: Buffer.concat(chunks).toString(), headers: new Headers(res.headers) }))
      })
      req.on('error', reject)
      req.end()
    })
  }
  const r = await fetch(base + path, {
    method, redirect: 'manual',
    headers: { Origin: base, 'Content-Type': 'application/json', ...(sendCookie && cookie ? { Cookie: cookie } : {}), ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(expectedSID ? { 'X-Auth-Session': expectedSID } : {}), ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const cookies = r.headers.getSetCookie()
  if (cookies.some(c => c.startsWith('new_api_refresh='))) cookie = cookies.find(c => c.startsWith('new_api_refresh=')).split(';')[0]
  const text = await r.text()
  let json; try { json = JSON.parse(text) } catch {}
  return { status: r.status, text, json, cookies, headers: r.headers }
}
const results = []
function passed(name) { results.push({ name, result: 'PASS' }) }
let r = await request('/api/setup')
assert.equal(r.status, 200)
assert.equal(r.json.data.root_init, false, 'fixture must be a fresh empty DB')
const password = randomBytes(28).toString('base64url')
r = await request('/api/setup', { method: 'POST', body: { username: 'cmhfixture', password, confirmPassword: password } })
assert.equal(r.json.success, true, 'official setup failed')
passed('fresh isolated official setup')
r = await request('/api/user/self')
assert.equal(r.status, 401)
passed('anonymous self denied')
r = await request('/api/user/login', { method: 'POST', body: { username: 'cmhfixture', password } })
assert.equal(r.json.success, true, 'official login failed')
assert.ok(r.json.data.access_token, 'fixture did not issue session; no MFA bypass attempted')
let token = r.json.data.access_token
const sid = r.json.data.session.sid
assert.ok(r.cookies.some(c => c.startsWith('new_api_refresh=') && /HttpOnly/i.test(c)))
assert.ok(r.cookies.some(c => c.startsWith('new_api_refresh=') && /SameSite=Strict/i.test(c)))
passed('official login issues HttpOnly SameSite session cookie')
r = await request('/api/user/self')
assert.equal(r.status, 401)
passed('cookie alone cannot authorize protected self')
r = await request('/api/user/auth/refresh', { method: 'POST', expectedSID: sid })
assert.equal(r.status, 200)
assert.equal(r.json.data.session.sid, sid)
token = r.json.data.access_token
passed('refresh cookie bootstraps in-memory access credential')
r = await request('/api/user/self', { token })
assert.equal(r.status, 200)
assert.equal(r.json.data.username, 'cmhfixture')
passed('Bearer obtained by refresh reaches official self')
r = await request('/cmh/whoami')
assert.equal(r.status, 200)
assert.match(r.text, /\/cmh\/static\/js\//)
assert.equal(r.headers.get('cache-control'), 'no-store')
passed('deep console link serves built SPA with correct assets')
const asset = r.text.match(/src="(\/cmh\/static\/js\/[^\"]+)"/)[1]
r = await request(asset)
assert.equal(r.status, 200)
assert.match(r.headers.get('content-type'), /javascript/)
passed('actual hashed JavaScript resource delivered')
r = await request('/cmh/static/js/nonexistent.js')
assert.equal(r.status, 404)
passed('missing JavaScript is not SPA fallback')
r = await request('/pricing', { headers: { Host: 'demo.brand.test' } })
assert.equal(r.status, 404)
passed('unimplemented brand page cannot expose main storefront')
r = await request('/cmh/whoami', { headers: { Host: 'demo.brand.test' } })
assert.equal(r.status, 404)
passed('brand host cannot reach operations console')
r = await request('/api/user/auth/logout', { method: 'POST', token, expectedSID: sid })
assert.equal(r.status, 200)
r = await request('/api/user/self', { token })
assert.equal(r.status, 401)
passed('official logout invalidates previous access credential')
r = await request('/api/user/auth/refresh', { method: 'POST', expectedSID: sid })
assert.equal(r.status, 401)
passed('logged-out refresh credential cannot be reused')
console.log(JSON.stringify({ scope: 'local HTTP fixture, NOT browser UI / NOT production TLS', checks: results, count: results.length }, null, 2))
