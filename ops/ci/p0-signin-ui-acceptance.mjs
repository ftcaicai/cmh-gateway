import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
const base='http://127.0.0.1:38080'
const pages=await(await fetch('http://127.0.0.1:39222/json')).json()
const page=pages.find(p=>p.type==='page')
assert.ok(page)
const ws=new WebSocket(page.webSocketDebuggerUrl)
await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject})
let seq=0
const pending=new Map()
ws.onmessage=e=>{const v=JSON.parse(e.data);const p=pending.get(v.id);if(p){pending.delete(v.id);v.error?p.reject(new Error(v.error.message)):p.resolve(v.result)}}
function cdp(method,params={}){return new Promise((resolve,reject)=>{const id=++seq;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}))})}
async function ev(expression){const r=await cdp('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error('UI evaluation failed (secret payload suppressed)');return r.result.value}
async function until(expression,label){const end=Date.now()+30000;while(Date.now()<end){if(await ev(expression))return;await new Promise(r=>setTimeout(r,100))}throw new Error('UI timeout: '+label)}
await cdp('Page.enable')
await cdp('Page.navigate',{url:base+'/api/status'})
await until("location.pathname==='/api/status'",'origin ready')
const password=randomBytes(28).toString('base64url')
assert.equal(await ev(`fetch('/api/setup',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'cmhuitest',password:${JSON.stringify(password)},confirmPassword:${JSON.stringify(password)}})}).then(r=>r.json()).then(j=>j.success)`),true)
await cdp('Page.navigate',{url:base+'/cmh/whoami'})
await until("location.pathname==='/sign-in' && !!document.querySelector('input[name=username]')",'real official sign-in form')
const officialScripts=await ev("[...document.scripts].some(s=>s.src.includes('/static/js/'))")
assert.equal(officialScripts,true)
await ev(`(()=>{const set=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set;for(const [name,value] of [['username','cmhuitest'],['password',${JSON.stringify(password)}]]){const el=document.querySelector('input[name='+name+']');set.call(el,value);el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}))}return true})()`)
await ev("document.querySelector('button[type=submit]').click()")
await until("location.pathname==='/cmh/whoami'",'official form submitted and redirect URL selected')
// Wait on the gateway's completed successful login, not a fixed UI sleep.
const established=await ev("fetch('/api/user/auth/refresh',{method:'POST'}).then(r=>r.json()).then(j=>!!j.data?.access_token)")
assert.equal(established,true)
const view=await ev("({path:location.pathname,usernameRendered:!!document.querySelector('[data-testid=auth-state]')?.textContent.includes('cmhuitest'),body:document.body.innerText.slice(0,700),scripts:[...document.scripts].map(s=>new URL(s.src||location.href).pathname).filter(p=>p.includes('/static/')).slice(0,5)})")
console.log(JSON.stringify({officialFormLoaded:true,officialFormLoginSucceeded:true,redirectPath:view.path,cmhUsernameRendered:view.usernameRendered,renderedShell:view.scripts.some(p=>p.startsWith('/cmh/'))?'cmh':'official',bodyPreview:view.body,limits:'local HTTP; snapshot after URL transition. Full CMH render must be separately awaited or its absence treated as failure.'},null,2))
try{await until("document.querySelector('[data-testid=auth-state]')?.textContent.includes('cmhuitest')",'CMH after official login');console.log('AUTOMATIC_CROSS_APP_RETURN=PASS')}
catch{console.log('AUTOMATIC_CROSS_APP_RETURN=FAIL_OFFICIAL_SPA_STILL_OWNS_DOCUMENT');process.exitCode=1}
ws.close()
