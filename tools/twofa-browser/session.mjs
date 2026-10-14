import crypto from 'node:crypto';
import { chromium } from 'playwright';
import { Server as ProxyServer, RequestError } from 'proxy-chain';
import { VIEWPORT, CALLBACK, CLIENT_ID, oauthRequest, callbackCode, credentialsFromTokens, totp, actionInput } from './core.mjs';

const allowedHost=host=>['auth.openai.com','chatgpt.com','challenges.cloudflare.com'].includes(host)||['.openai.com','.oaistatic.com','.oaiusercontent.com','.chatgpt.com'].some(suffix=>host.endsWith(suffix));
const safeCodes=new Set(['authorization_rejected','browser_unavailable','proxy_connect_failed','security_verification_required','interaction_required','login_timeout','password_rejected','totp_rejected','token_exchange_failed','account_identity_mismatch','invalid_oauth_callback']);

export class LoginSession {
 constructor(data,options={}){
  this.id=crypto.randomBytes(24).toString('hex');this.data=data;this.options=options;
  this.status='running';this.stage='browser_starting';this.code='';this.httpStatus=0;
  this.closed=false;this.manual=false;this.busy=false;this.actionCounts={};this.started=Date.now();this.credential=null;
  this.oauth=oauthRequest(options.authOrigin||'https://auth.openai.com');
 }
 progress(){return {status:this.status,stage:this.stage,code:this.code,http_status:this.httpStatus,timezone:this.data.timezone, ...(this.credential?{credential:this.credential}:{})};}
 wait(code='interaction_required',stage=this.stage){if(this.closed||this.credential||this.exchanging||this.status==='failed')return;this.status='waiting_interaction';this.code=code;this.stage=stage;this.manual=true;}
 fail(code){if(this.closed||this.credential)return;this.status='failed';this.code=safeCodes.has(code)?code:'browser_unavailable';this.data.password='';this.data.totp='';void this.closeBrowser();}
 async start(){
  try{
   const fixtureHost=this.options.fixture?new URL(this.options.authOrigin).hostname:null;
   // Resolve destination names at the selected SOCKS proxy, matching the Go
   // proxy dialer. Local DNS must not be a hidden dependency of remote login.
   const upstream=new URL(this.data.proxy);
   if(upstream.protocol==='socks5:')upstream.protocol='socks5h:';
   this.bridge=new ProxyServer({port:0,host:'127.0.0.1',verbose:false,prepareRequestFunction:({hostname,port,request})=>{
    // Chromium does not route-intercept the second hop of an HTTP redirect.
    // Terminate the exact loopback callback here, inside this session's bridge;
    // never forward it to the selected upstream or a host loopback listener.
    const callback=new URL(CALLBACK);
    let target;try{target=new URL(request.url);}catch{}
    if(target?.origin===callback.origin&&target.pathname===callback.pathname){
     return {requestAuthentication:false,customResponseFunction:async()=>{
      void this.complete(target.href);
      return {statusCode:200,headers:{'Content-Type':'text/html','Cache-Control':'no-store'},body:'<!doctype html><title>Authorization received</title><p>Authorization received. Return to the import window.</p>'};
     }};
    }
    if((!allowedHost(hostname)&&hostname!==fixtureHost)||(port!==443&&!this.options.fixture))throw new RequestError('Authentication destination unavailable',403);
    return {requestAuthentication:false,upstreamProxyUrl:upstream.href,ignoreUpstreamProxyCertificate:false};
   }});
   this.bridge.on('requestFailed',()=>{this.proxyFailed=true;});
   this.bridge.on('tunnelConnectResponded',({response})=>{if(response.statusCode>=400)this.proxyFailed=true;});
   await this.bridge.listen();if(this.closed)return await this.closeBrowser();
   this.browser=await chromium.launch({headless:this.options.headless??false,executablePath:this.options.executablePath,proxy:{server:`http://127.0.0.1:${this.bridge.port}`,bypass:'<-loopback>'},args:['--disable-background-networking','--disable-component-update','--disable-sync'],timeout:30000});
   if(this.closed)return await this.closeBrowser();
   this.context=await this.browser.newContext({viewport:VIEWPORT,timezoneId:this.data.timezone,locale:'en-US',acceptDownloads:false,serviceWorkers:'block'});
   this.context.setDefaultTimeout(3500);
   await this.context.route('**/*',async route=>{
    const request=route.request(),u=new URL(request.url());
    if(u.origin===new URL(CALLBACK).origin&&u.pathname===new URL(CALLBACK).pathname){
     if(request.isNavigationRequest())void this.complete(u.href);
     return route.fulfill({status:200,contentType:'text/html',body:'<!doctype html><title>Authorization received</title><p>Authorization received. You can return to the import window.</p>'});
    }
    if((u.protocol==='https:'&&allowedHost(u.hostname))||(this.options.fixture&&u.origin===this.options.authOrigin))return route.continue();
    return route.abort('blockedbyclient');
   });
   this.page=await this.context.newPage();
   this.page.on('dialog',dialog=>void dialog.dismiss().catch(()=>{}));
   this.page.on('popup',popup=>void popup.close().catch(()=>{}));
   this.page.on('response',response=>{
    if(response.request().isNavigationRequest()&&response.frame()===this.page.mainFrame()){
     this.httpStatus=response.status();
     if(response.status()===407||(response.status()>=590&&response.status()<=599))return this.fail('proxy_connect_failed');
     if(response.headers()['cf-mitigated']==='challenge')this.wait('security_verification_required','security_verification');
     else if(response.status()===403)this.wait('authorization_rejected','authorization');
    }
   });
   this.stage='navigation';
   try{await this.page.goto(this.oauth.url,{waitUntil:'domcontentloaded',timeout:45000});}
   catch(e){if(this.closed||this.credential)return;if(this.proxyFailed||/ERR_(PROXY|TUNNEL|SOCKS)/.test(String(e)))return this.fail('proxy_connect_failed');this.wait('interaction_required','navigation');}
   if(this.closed||this.status==='failed'||this.status==='succeeded')return;
   this.timer=setInterval(()=>void this.tick(),900);
   await this.tick();
  }catch(e){if(!this.closed)this.fail(this.proxyFailed?'proxy_connect_failed':'browser_unavailable');}
 }
 async tick(){
  if(this.closed||this.busy||this.exchanging||this.status==='failed'||this.status==='succeeded')return;
  if(Date.now()-this.started>20*60*1000)return this.fail('login_timeout');
  if(!this.page||this.page.isClosed())return this.fail('browser_unavailable');
  if(this.manual)return;
  this.busy=true;
  try{
   if(new URL(this.page.url()).origin!==(this.options.authOrigin||'https://auth.openai.com'))return this.wait('interaction_required','authorization');
   const body=await this.page.locator('body').innerText({timeout:2000}).catch(()=>'');
   // Challenges are displayed for the human; no solver, checkbox clicking,
   // browser fingerprint overrides or clearance-cookie transfer is performed.
   if(/verify you are human|verifying you are human|checking your browser|just a moment|security verification|安全验证|验证您是人类/i.test(body))return this.wait('security_verification_required','security_verification');
   if(/incorrect password|wrong password|invalid password|密码错误/i.test(body))return this.wait('password_rejected','password');
   if(/invalid code|incorrect code|验证码错误/i.test(body))return this.wait('totp_rejected','totp');
   const candidates=[
    ['password','input[type="password"]',()=>this.data.password],
    ['totp','input[autocomplete="one-time-code"],input[name="code"],input[name="totp"]',()=>totp(this.data.totp)],
    ['email','input[type="email"],input[name="email"],input[name="username"]',()=>this.data.email],
   ];
   for(const [stage,selector,value] of candidates){
    const input=this.page.locator(selector).filter({visible:true}).first();
    if(!await input.isVisible())continue;
    this.stage=stage;
    const signature=stage+':'+new URL(this.page.url()).pathname;
    if((this.actionCounts[signature]||0)>=1)return this.wait('interaction_required',stage);
    this.actionCounts[signature]=(this.actionCounts[signature]||0)+1;
    await input.fill(value());
    const button=this.page.getByRole('button',{name:/^(continue|next|log in|sign in|submit|verify code|verify|继续|下一步|登录|验证)$/i}).filter({visible:true});
    if(await button.count()!==1)return this.wait('interaction_required',stage);
    this.status='running';this.code='';await button.click();
    await this.page.waitForTimeout(1200);return;
   }
   this.wait('interaction_required','authorization');
  }catch(e){if(!this.closed&&this.status!=='succeeded')this.wait(e.message==='totp_rejected'?'totp_rejected':'interaction_required');}
  finally{this.busy=false;}
 }
 async complete(value){
  if(this.exchanging||this.credential||this.closed)return;
  this.exchanging=true;this.manual=true;this.status='running';this.stage='token_exchange';this.code='';
  try{
   const code=callbackCode(value,this.oauth.state);
   const response=await this.context.request.post((this.options.authOrigin||'https://auth.openai.com')+'/oauth/token',{form:{grant_type:'authorization_code',client_id:CLIENT_ID,code,code_verifier:this.oauth.verifier,redirect_uri:CALLBACK},timeout:45000,maxRedirects:0});
   this.httpStatus=response.status();if(!response.ok())throw new Error('token_exchange_failed');
   const credential=credentialsFromTokens(await response.json(),this.data.email,this.oauth.nonce);
   if(this.closed||this.status==='failed')return;
   this.credential=credential;
   this.data.password='';this.data.totp='';this.oauth.verifier='';this.oauth.state='';
   this.stage='succeeded';this.status='succeeded';this.code='';await this.closeBrowser();
  }catch(e){this.fail(safeCodes.has(e.message)?e.message:'token_exchange_failed');}
 }
 async frame(){if(!this.page||this.closed||this.page.isClosed())throw new Error('not_ready');return this.page.screenshot({type:'jpeg',quality:65,timeout:6000});}
 async input(raw){
  const action=actionInput(raw);
  if(!this.page||this.page.isClosed()||this.closed||this.exchanging)throw new Error('not_ready');
  if(this.busy)throw new Error('busy');
  this.manual=true;this.busy=true;
  try{
   if(action.type==='click')await this.page.mouse.click(action.x,action.y);
   else if(action.type==='text')await this.page.keyboard.insertText(action.text);
   else if(action.type==='key')await this.page.keyboard.press(action.key);
   else if(action.type==='wheel')await this.page.mouse.wheel(0,action.delta);
  }finally{this.busy=false;}
 }
 resume(){if(this.closed||this.exchanging||this.status==='failed'||this.status==='succeeded')throw new Error('not_ready');this.manual=false;this.status='running';this.code='';}
 async closeBrowser(){clearInterval(this.timer);await this.browser?.close().catch(()=>{});await this.bridge?.close(true).catch(()=>{});}
 async close(){this.closed=true;this.data.password='';this.data.totp='';this.credential=null;this.oauth.verifier='';await this.closeBrowser();}
}
