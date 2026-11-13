import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
const base = 'http://127.0.0.1:38080'
const pages = await (await fetch('http://127.0.0.1:39222/json')).json()
const page = pages.find(p => p.type === 'page')
assert.ok(page)
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject })
let sequence = 0
const pending = new Map()
ws.onmessage = event => { const v = JSON.parse(event.data); if (v.id && pending.has(v.id)) { const {resolve,reject} = pending.get(v.id); pending.delete(v.id); if (v.error) reject(new Error(v.error.message)); else resolve(v.result) } }
function cdp(method, params = {}) { return new Promise((resolve,reject) => { const id = ++sequence; pending.set(id,{resolve,reject}); ws.send(JSON.stringify({id,method,params})) }) }
async function evaluate(expression) { const r = await cdp('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true}); if (r.exceptionDetails) throw new Error('browser evaluation failed (details suppressed to protect fixture secrets)'); return r.result.value }
async function until(expression, label) { const end = Date.now()+20000; while (Date.now()<end) { if (await evaluate(expression)) return; await new Promise(r=>setTimeout(r,100)) }; throw new Error('browser acceptance timeout: '+label) }
await cdp('Page.enable')
await cdp('Page.navigate',{url:base+'/api/status'})
await until("location.pathname === '/api/status'",'initial document')
const password=randomBytes(28).toString('base64url')
const setup = await evaluate(`(async()=>{const r=await fetch('/api/setup',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'cmhbrowser',password:${JSON.stringify(password)},confirmPassword:${JSON.stringify(password)}})});return (await r.json()).success})()`)
assert.equal(setup,true)
await cdp('Page.navigate',{url:base+'/cmh/whoami'})
await until("location.pathname === '/sign-in'",'anonymous sign-in redirect')
assert.equal(await evaluate("new URL(location.href).searchParams.get('redirect')"),'/cmh/whoami')
const login = await evaluate(`(async()=>{const r=await fetch('/api/user/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'cmhbrowser',password:${JSON.stringify(password)}})});const j=await r.json();return j.success && !!j.data?.access_token})()`)
assert.equal(login,true)
await cdp('Page.navigate',{url:base+'/cmh/whoami'})
await until("document.querySelector('[data-testid=auth-state]')?.textContent.includes('cmhbrowser')",'cold application refresh and current user')
await cdp('Page.reload')
await until("document.querySelector('[data-testid=auth-state]')?.textContent.includes('cmhbrowser')",'reload session persistence')
assert.equal(await evaluate("document.cookie.includes('new_api_refresh=')"),false)
const tokenStorage = await evaluate("[...Object.values(localStorage),...Object.values(sessionStorage)].some(v => /eyJ[A-Za-z0-9_-]+\\.eyJ/.test(v))")
assert.equal(tokenStorage,false)
await evaluate("fetch('/api/user/auth/logout',{method:'POST',headers:{'Content-Type':'application/json'}}).then(r=>r.status)")
await cdp('Page.reload')
await until("location.pathname === '/sign-in'",'logout denial')
console.log(JSON.stringify({browser:'Chrome headless real DOM/CDP',checks:['anonymous redirect points to /sign-in with local CMH return path','official API login establishes refresh cookie','cold CMH document exchanges cookie for Bearer and renders username','full document reload preserves identity via official session','refresh cookie not script-readable','no JWT in web storage','official logout then reload redirects to sign-in'],passed:7,limits:['official sign-in UI is not built: login performed via official API, not UI form','automatic official SPA login callback to CMH remains unverified','loopback HTTP does not verify production TLS/Secure/CSRF']},null,2))
ws.close()
