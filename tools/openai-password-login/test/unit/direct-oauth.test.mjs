import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import os from 'node:os'
import {runDirectOAuth} from '../../scripts/codex-login/direct-oauth.mjs'
import {validateCallback,observeAuthResponse} from '../../scripts/codex-login/hooks.mjs'

async function fixture(t,kind='normal') {
 const directory=await fs.mkdtemp(path.join(os.tmpdir(),'direct-oauth-test-'))
 t.after(()=>fs.rm(directory,{recursive:true,force:true}))
 const calls=[],stages=[];let authorization
 class ProtocolClient {
  jar={value:()=> 'synthetic-device',merge:()=>{},headerFor:()=> 'synthetic-cookie'}
  async follow(url){
   calls.push({type:'follow',url})
   if (kind==='challenge') throw new Error('security_check')
   const u=new URL(url)
   if(u.pathname==='/oauth/authorize') {authorization=u;return {finalUrl:'https://auth.openai.com/log-in',last:{text:''}}}
   return {finalUrl:url,last:{text:''}}
  }
  async getJson(method,url,options){calls.push({type:'username',url,method,options});return {data:{continue_url:'https://auth.openai.com/log-in/password'}}}
 }
 const runtime={
  ProtocolClient,
  createSentinelHeaders:async()=>({}),
  verifyPassword:async(client,args)=>{calls.push({type:'password',auth:args.authBase});if(kind==='password')throw new Error('password_rejected');return {page:{type:'mfa_challenge'}}},
  completeTotpMfaIfNeeded:async(client,args)=>{calls.push({type:'mfa',auth:args.authBase});return kind==='email'?{page:{type:'email_otp_send'}}:{continue_url:'https://auth.openai.com/workspace'}},
  finishCodexOauth:async(client,options,current,oauth)=>({callbackUrl:'http://localhost:1455/auth/callback?code=synthetic-code&state='+(kind==='state'?'wrong':oauth.state),...oauth}),
  buildSub2apiOauthExport:async args=>{calls.push({type:'token',auth:args.authBase});return {data:{accounts:[{credentials:{access_token:'synthetic-token'}}]}}},
 }
 const transport={configure:async(proxy)=>calls.push({type:'proxy',proxy})}
 const input={email:'test@example.test',password:'synthetic-password',mfaSecret:'JBSWY3DPEHPK3PXP',proxyUrl:'',outputPath:path.join(directory,'oauth.json')}
 const run=()=>runDirectOAuth(runtime,{validateCallback,observeAuthResponse,reportLoginStage:s=>stages.push(s)},{transport,input})
 return {run,calls,input,stages,getAuthorization:()=>authorization}
}
test('direct OAuth goes to the official grant entry and retains PKCE, MFA and the selected exit',async t=>{
 const f=await fixture(t);await f.run()
 assert.equal(f.calls.filter(c=>c.type==='proxy').length,1)
 assert.equal(f.calls.find(c=>c.type==='proxy').proxy,null)
 assert.equal(f.calls.some(c=>c.url?.includes('chatgpt.com')),false)
 assert.equal(f.calls.filter(c=>c.type==='password').length,1)
 assert.equal(f.calls.filter(c=>c.type==='mfa').length,1)
 assert.equal(f.getAuthorization().searchParams.get('code_challenge_method'),'S256')
 assert.equal(f.getAuthorization().searchParams.get('redirect_uri'),'http://localhost:1455/auth/callback')
 assert.equal(f.calls.at(-1).type,'token')
 const p=await fixture(t);p.input.proxyUrl='socks5h://selected.example.test:1080';await p.run();assert.equal(p.calls[0].proxy,p.input.proxyUrl)
})
test('direct OAuth stops on real challenge, rejected password, email verification and wrong state',async t=>{
 for(const [kind,reason] of [['challenge','security_check'],['password','password_rejected'],['email','email_verification_required'],['state','callback_invalid']]){
  const f=await fixture(t,kind);await assert.rejects(f.run, new RegExp(reason))
  assert.equal(f.calls.some(c=>c.type==='token'),false)
  assert.equal(f.calls.filter(c=>c.type==='password').length<=1,true)
  assert.equal(f.calls.some(c=>c.url?.includes('chatgpt.com')),false)
  await assert.rejects(fs.stat(f.input.outputPath),{code:'ENOENT'})
 }
})
