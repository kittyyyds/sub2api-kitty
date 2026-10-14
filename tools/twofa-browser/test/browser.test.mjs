import {test} from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import net from 'node:net';
import crypto from 'node:crypto';
import {once} from 'node:events';
import {LoginSession} from '../session.mjs';
import {CALLBACK,CLIENT_ID,totp} from '../core.mjs';

const listen=async server=>{server.listen(0,'127.0.0.1');await once(server,'listening');return server.address().port;};
async function until(fn,message,timeout=25000){const end=Date.now()+timeout;while(Date.now()<end){if(await fn())return;await new Promise(r=>setTimeout(r,50));}throw new Error(message);}
const jwt=x=>`fixture.${Buffer.from(JSON.stringify(x)).toString('base64url')}.fixture`;
function tokenResponse(email,nonce){const now=Math.floor(Date.now()/1000);return {access_token:jwt({'https://api.openai.com/auth':{chatgpt_account_id:'fixture-account'}}),refresh_token:'fixture-refresh',id_token:jwt({email,nonce,iss:'https://auth.openai.com',aud:CLIENT_ID,iat:now,exp:now+3600}),expires_in:3600};}
function fixtureAuth({challenge=false,mismatch=false,tokenError=false}={}){
 const seen=[],environments=[];let auth,stage=challenge?'challenge':'email',tokenSeen=false;
 const server=http.createServer(async(req,res)=>{
  const u=new URL(req.url,'http://fixture');seen.push(u.pathname);let raw='';for await(const c of req)raw+=c;
  const redirect=path=>{res.writeHead(302,{location:path});res.end();};
  if(u.pathname==='/oauth/authorize'){auth=u.searchParams;res.setHeader('Set-Cookie','fixture_session=bound; HttpOnly; Path=/');return redirect('/form');}
  if(u.pathname==='/environment'){environments.push(JSON.parse(raw));res.end('ok');return;}
  if(u.pathname==='/form'){
   if(stage==='challenge')res.writeHead(403,{'Content-Type':'text/html','cf-mitigated':'challenge'});
   else res.writeHead(200,{'Content-Type':'text/html'});
   const content=stage==='challenge'?'<p>Fixture only: verify you are human</p><form action="/verify" method="post"><button>Finish fixture verification</button></form>':`<form action="/submit" method="post"><input ${stage==='password'?'type="password"':stage==='totp'?'autocomplete="one-time-code" name="code"':'type="email"'} name="value"><button>Continue</button></form>`;
   res.end(`<!doctype html><title>Synthetic login fixture</title>${content}<script>fetch('/environment',{method:'POST',body:JSON.stringify({timezone:Intl.DateTimeFormat().resolvedOptions().timeZone,offset:new Date('2026-07-01T12:00:00Z').getTimezoneOffset()})})</script>`);return;
  }
  if(u.pathname==='/verify'){stage='email';return redirect('/form');}
  if(u.pathname==='/submit'){
   const form=new URLSearchParams(raw),value=form.get('value')||form.get('code');
   const expected=stage==='email'?'fixture@example.invalid':stage==='password'?'fixture-password':totp('GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ');
   if(value!==expected){res.writeHead(400);res.end('Invalid fixture field');return;}
   if(stage==='totp')return redirect(`${CALLBACK}?code=fixture-code&state=${auth.get('state')}`);
   stage=stage==='email'?'password':'totp';return redirect('/form');
  }
  if(u.pathname==='/oauth/token'){
   tokenSeen=true;const form=new URLSearchParams(raw);
   assert.equal(req.headers.cookie,'fixture_session=bound');
   assert.equal(form.get('redirect_uri'),CALLBACK);assert.equal(form.get('client_id'),CLIENT_ID);
   assert.equal(crypto.createHash('sha256').update(form.get('code_verifier')).digest('base64url'),auth.get('code_challenge'));
   res.writeHead(tokenError?403:200,{'Content-Type':'application/json'});
   res.end(JSON.stringify(tokenError?{error:'fixture_only'}:tokenResponse(mismatch?'other@example.invalid':'fixture@example.invalid',auth.get('nonce'))));return;
  }
  res.writeHead(404);res.end();
 });
 return {server,seen,environments,get tokenSeen(){return tokenSeen;}};
}
function fixtureProxy(protocol,authPort){
 let accepted=0;const sockets=new Set();
 const track=socket=>{sockets.add(socket);socket.on('close',()=>sockets.delete(socket));socket.on('error',()=>{});return socket;};
 let server;
 if(protocol==='http'){
  server=http.createServer((req,res)=>{
   if(req.headers['proxy-authorization']!==`Basic ${Buffer.from('fixture:pass').toString('base64')}`){res.writeHead(407);res.end();return;}
   const u=new URL(req.url);if(u.hostname!=='auth.fixture.invalid'){res.writeHead(403);res.end();return;}
   accepted++;const headers={...req.headers};delete headers['proxy-authorization'];
   const upstream=http.request({host:'127.0.0.1',port:authPort,path:u.pathname+u.search,method:req.method,headers},response=>{res.writeHead(response.statusCode,response.headers);response.pipe(res);});
   upstream.on('error',()=>{res.writeHead(502);res.end();});req.pipe(upstream);
  });
  server.on('connect',(req,socket,head)=>{
   if(req.headers['proxy-authorization']!==`Basic ${Buffer.from('fixture:pass').toString('base64')}`||!req.url.startsWith('auth.fixture.invalid:'))return socket.end('HTTP/1.1 407 Proxy Authentication Required\r\n\r\n');
   const upstream=track(net.connect(authPort,'127.0.0.1',()=>{accepted++;socket.write('HTTP/1.1 200 Connection Established\r\n\r\n');if(head.length)upstream.write(head);socket.pipe(upstream);upstream.pipe(socket);}));
   socket.on('close',()=>upstream.destroy());upstream.on('close',()=>socket.destroy());
  });
 }else{
  server=net.createServer(socket=>{
   let buffer=Buffer.alloc(0),phase=0;
   const receive=chunk=>{
    buffer=Buffer.concat([buffer,chunk]);
    if(phase===0){if(buffer.length<2||buffer.length<2+buffer[1])return;buffer=buffer.subarray(2+buffer[1]);socket.write(Buffer.from([5,2]));phase=1;}
    if(phase===1){if(buffer.length<2)return;const ulen=buffer[1];if(buffer.length<3+ulen)return;const plen=buffer[2+ulen];if(buffer.length<3+ulen+plen)return;
     if(buffer.subarray(2,2+ulen).toString()!=='fixture'||buffer.subarray(3+ulen,3+ulen+plen).toString()!=='pass')return socket.end(Buffer.from([1,1]));
     buffer=buffer.subarray(3+ulen+plen);socket.write(Buffer.from([1,0]));phase=2;
    }
    if(phase===2){if(buffer.length<5)return;const domain=buffer[3]===3,length=domain?buffer[4]:buffer[3]===1?4:16,offset=domain?5:4;if(buffer.length<offset+length+2)return;
     const host=buffer.subarray(offset,offset+length).toString();if(!domain||host!=='auth.fixture.invalid')return socket.end(Buffer.from([5,2,0,1,0,0,0,0,0,0]));
     const extra=buffer.subarray(offset+length+2);phase=3;socket.removeListener('data',receive);
     const upstream=track(net.connect(authPort,'127.0.0.1',()=>{accepted++;socket.write(Buffer.from([5,0,0,1,127,0,0,1,0,0]));if(extra.length)upstream.write(extra);socket.pipe(upstream);upstream.pipe(socket);}));
     socket.on('close',()=>upstream.destroy());upstream.on('close',()=>socket.destroy());
    }
   };socket.on('data',receive);
  });
 }
 server.on('connection',track);
 return {server,get accepted(){return accepted;},async close(){for(const s of sockets)s.destroy();await new Promise(r=>server.close(r));}};
}
const credentials={email:'fixture@example.invalid',password:'fixture-password',totp:'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ'};
for(const [protocol,timezone,offset,challenge] of [['http','America/Los_Angeles',420,true],['socks5','Asia/Tokyo',-540,false]]){
 test(`real browser preserves ${protocol} authenticated proxy, ${timezone}, cookies and PKCE through login`,{timeout:70000},async()=>{
  const auth=fixtureAuth({challenge}),port=await listen(auth.server),proxy=fixtureProxy(protocol,port),proxyPort=await listen(proxy.server);
  const session=new LoginSession({...credentials,proxy:`${protocol}://fixture:pass@127.0.0.1:${proxyPort}`,timezone},{fixture:true,authOrigin:`http://auth.fixture.invalid:${port}`,headless:true});
  try{
   await session.start();
   if(challenge){
    await until(()=>session.status==='waiting_interaction','challenge should pause');
    assert.equal(session.code,'security_verification_required');assert.equal(auth.seen.includes('/submit'),false);
    assert.ok((await session.frame()).length>1000);
    // Only our synthetic local challenge is completed by this test.
    const box=await session.page.getByRole('button',{name:'Finish fixture verification'}).boundingBox();
    await session.input({type:'click',x:box.x+5,y:box.y+5});await session.page.locator('input[type="email"]').waitFor();session.resume();
   }
   await until(()=>['succeeded','failed'].includes(session.status),'login did not finish').catch(async e=>{throw new Error(e.message+JSON.stringify({progress:session.progress(),paths:auth.seen,body:await session.page.locator('body').innerText()}));});
   assert.equal(session.status,'succeeded',JSON.stringify(session.progress()));
   assert.equal(session.credential.email,credentials.email);assert.equal(auth.tokenSeen,true);
   assert.ok(proxy.accepted>=2);assert.ok(auth.environments.length>=3);
   assert.ok(auth.environments.every(e=>e.timezone===timezone&&e.offset===offset));
   assert.equal(session.data.password,'');assert.equal(session.data.totp,'');
   assert.equal(session.browser.isConnected(),false);
  }finally{await session.close();await proxy.close();auth.server.closeAllConnections();await new Promise(r=>auth.server.close(r));}
 });
}
for(const [scenario,expected] of [[{mismatch:true},'account_identity_mismatch'],[{tokenError:true},'token_exchange_failed']]){
 test(`failed OAuth exchange cannot produce an import: ${expected}`,{timeout:60000},async()=>{
  const auth=fixtureAuth(scenario),port=await listen(auth.server),proxy=fixtureProxy('http',port),p=await listen(proxy.server);
  const session=new LoginSession({...credentials,proxy:`http://fixture:pass@127.0.0.1:${p}`,timezone:'UTC'},{fixture:true,authOrigin:`http://auth.fixture.invalid:${port}`,headless:true});
  try{await session.start();await until(()=>session.status==='failed','failure expected');assert.equal(session.code,expected);assert.equal(session.credential,null);}
  finally{await session.close();await proxy.close();auth.server.closeAllConnections();await new Promise(r=>auth.server.close(r));}
 });
}
test('unreachable selected proxy fails without a direct login', {timeout:55000},async()=>{
 const auth=fixtureAuth(),port=await listen(auth.server);
 const session=new LoginSession({...credentials,proxy:'http://127.0.0.1:1',timezone:'UTC'},{fixture:true,authOrigin:`http://auth.fixture.invalid:${port}`,headless:true});
 try{await session.start();assert.notEqual(session.status,'succeeded');assert.equal(auth.seen.length,0);assert.equal(session.code,'proxy_connect_failed');}
 finally{await session.close();auth.server.closeAllConnections();await new Promise(r=>auth.server.close(r));}
});
