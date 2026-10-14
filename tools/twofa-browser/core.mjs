import crypto from 'node:crypto';

export const VIEWPORT={width:1024,height:768};
export const CALLBACK='http://localhost:1455/auth/callback';
export const CLIENT_ID='app_EMoamEEZ73f0CkXaXp7hrann';
export function validTimezone(value){
 if(typeof value!=='string'||value.length>80)return false;
 try{new Intl.DateTimeFormat('en',{timeZone:value}).format();return true;}catch{return false;}
}
export function validateInput(data){
 if(!data||typeof data.email!=='string'||!/^\S+@\S+\.\S+$/.test(data.email)||data.email.length>320)throw new Error('invalid_input');
 for(const k of ['password','totp'])if(typeof data[k]!=='string'||!data[k]||data[k].length>4096)throw new Error('invalid_input');
 if(!validTimezone(data.timezone))throw new Error('invalid_timezone');
 const p=new URL(data.proxy);if(!['http:','https:','socks5:','socks5h:'].includes(p.protocol)||!p.hostname)throw new Error('invalid_proxy');
 return {...data,email:data.email.toLowerCase()};
}
export function oauthRequest(authOrigin){
 const verifier=crypto.randomBytes(48).toString('base64url'),state=crypto.randomBytes(24).toString('base64url'),nonce=crypto.randomBytes(24).toString('base64url');
 const url=new URL('/oauth/authorize',authOrigin);
 url.search=new URLSearchParams({client_id:CLIENT_ID,redirect_uri:CALLBACK,response_type:'code',scope:'openid profile email offline_access',code_challenge:crypto.createHash('sha256').update(verifier).digest('base64url'),code_challenge_method:'S256',state,nonce,id_token_add_organizations:'true',codex_cli_simplified_flow:'true'}).toString();
 return {url:url.href,verifier,state,nonce};
}
export function callbackCode(value,state){
 const url=new URL(value),expected=new URL(CALLBACK);
 if(url.origin!==expected.origin||url.pathname!==expected.pathname||url.searchParams.get('state')!==state||url.searchParams.has('error'))throw new Error('invalid_oauth_callback');
 const code=url.searchParams.get('code');if(!code||code.length>8192)throw new Error('invalid_oauth_callback');return code;
}
export function totp(secret,now=Date.now()){
 const clean=secret.toUpperCase().replace(/[\s=-]/g,'');if(!/^[A-Z2-7]{16,256}$/.test(clean))throw new Error('totp_rejected');
 let bits='';for(const c of clean)bits+='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'.indexOf(c).toString(2).padStart(5,'0');
 const key=Buffer.from(bits.match(/.{8}/g).map(b=>parseInt(b,2))),counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(Math.floor(now/30000)));
 const hash=crypto.createHmac('sha1',key).update(counter).digest(),offset=hash[19]&15;
 return ((hash.readUInt32BE(offset)&0x7fffffff)%1000000).toString().padStart(6,'0');
}
export function credentialsFromTokens(tokens,email,nonce){
 for(const key of ['access_token','refresh_token','id_token'])if(typeof tokens[key]!=='string'||!tokens[key])throw new Error('token_exchange_failed');
 let claims,access;
 try{claims=JSON.parse(Buffer.from(tokens.id_token.split('.')[1],'base64url'));access=JSON.parse(Buffer.from(tokens.access_token.split('.')[1],'base64url'));}catch{throw new Error('token_exchange_failed');}
 // Tokens arrive directly from the TLS-protected authorization server exchange,
 // never from a browser form, pasted callback or an untrusted client.
 if(typeof claims.email!=='string'||claims.email.toLowerCase()!==email)throw new Error('account_identity_mismatch');
 if(claims.nonce!==nonce)throw new Error('invalid_oauth_callback');
 const audience=Array.isArray(claims.aud)?claims.aud:[claims.aud],now=Math.floor(Date.now()/1000);
 if(typeof claims.iss!=='string'||claims.iss.replace(/\/$/,'')!=='https://auth.openai.com'||!audience.includes(CLIENT_ID)||!Number.isFinite(claims.exp)||claims.exp<=now||!Number.isFinite(claims.iat)||claims.iat>now+60)throw new Error('token_exchange_failed');
 if(claims.nbf!==undefined&&(!Number.isFinite(claims.nbf)||claims.nbf>now+60))throw new Error('token_exchange_failed');
 const auth=access['https://api.openai.com/auth']||claims['https://api.openai.com/auth']||{};
 const id=auth.chatgpt_account_id||'';
 return {access_token:tokens.access_token,refresh_token:tokens.refresh_token,id_token:tokens.id_token,email:claims.email,chatgpt_account_id:id,account_id:id,chatgpt_user_id:auth.chatgpt_user_id||auth.user_id||claims.sub||'',client_id:CLIENT_ID,expires_at:Math.floor(Date.now()/1000)+Number(tokens.expires_in||3600)};
}
export function actionInput(data){
 if(!data||!['click','text','key','wheel'].includes(data.type))throw new Error('invalid_input');
 if(data.type==='click'&&(!Number.isFinite(data.x)||!Number.isFinite(data.y)||data.x<0||data.y<0||data.x>=VIEWPORT.width||data.y>=VIEWPORT.height))throw new Error('invalid_input');
 if(data.type==='text'&&(typeof data.text!=='string'||data.text.length>4096))throw new Error('invalid_input');
 if(data.type==='key'&&!['Enter','Tab','Shift+Tab','Backspace','Delete','Escape','ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Control+a'].includes(data.key))throw new Error('invalid_input');
 if(data.type==='wheel'&&(!Number.isFinite(data.delta)||Math.abs(data.delta)>1200))throw new Error('invalid_input');
 return data;
}
