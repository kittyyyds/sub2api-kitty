import {test} from 'node:test';
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import {CLIENT_ID,validateInput,oauthRequest,callbackCode,totp,credentialsFromTokens,actionInput} from '../core.mjs';
const jwt=claims=>`fixture.${Buffer.from(JSON.stringify(claims)).toString('base64url')}.fixture`;
export function fixtureTokens(email,nonce){const now=Math.floor(Date.now()/1000);return {access_token:jwt({'https://api.openai.com/auth':{chatgpt_account_id:'fixture-account'}}),refresh_token:'fixture-refresh',id_token:jwt({email,nonce,iss:'https://auth.openai.com',aud:CLIENT_ID,exp:now+3600,iat:now}),expires_in:3600};}
test('PKCE and callback state bind one authorization transaction',()=>{
 const a=oauthRequest('https://auth.example.invalid'),b=oauthRequest('https://auth.example.invalid');
 assert.notEqual(a.state,b.state);assert.notEqual(a.nonce,b.nonce);
 assert.equal(new URL(a.url).searchParams.get('code_challenge'),crypto.createHash('sha256').update(a.verifier).digest('base64url'));
 assert.equal(callbackCode(`http://localhost:1455/auth/callback?code=fixture&state=${a.state}`,a.state),'fixture');
 for(const url of [`http://localhost:1455/auth/callback?code=x&state=${b.state}`,`https://attacker.invalid/auth/callback?code=x&state=${a.state}`,`http://localhost:1455/auth/callback?error=no&state=${a.state}`])assert.throws(()=>callbackCode(url,a.state));
});
test('TOTP uses Unix time, independent of local timezone',()=>{
 assert.equal(totp('GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ',59000),'287082');
 assert.throws(()=>totp('not a secret!'));
});
test('only expected identity, nonce, issuer, audience and unexpired token response are accepted',()=>{
 const tokens=fixtureTokens('fixture@example.invalid','nonce');
 assert.equal(credentialsFromTokens(tokens,'fixture@example.invalid','nonce').account_id,'fixture-account');
 assert.throws(()=>credentialsFromTokens(tokens,'other@example.invalid','nonce'),/account_identity_mismatch/);
 assert.throws(()=>credentialsFromTokens(tokens,'fixture@example.invalid','other'),/invalid_oauth_callback/);
 const claims=JSON.parse(Buffer.from(tokens.id_token.split('.')[1],'base64url'));
 for(const override of [{iss:'https://untrusted.invalid'},{aud:'wrong-client'},{exp:1},{iat:Date.now()},{nonce:undefined}])assert.throws(()=>credentialsFromTokens({...tokens,id_token:jwt({...claims,...override})},'fixture@example.invalid','nonce'));
});
test('proxy, timezone and remote controls have bounded schemas',()=>{
 const input={email:'fixture@example.invalid',password:'p',totp:'s',proxy:'socks5://fixture:pass@127.0.0.1:1234',timezone:'Asia/Tokyo'};
 assert.equal(validateInput(input).timezone,'Asia/Tokyo');
 for(const override of [{proxy:'file:///etc/passwd'},{timezone:'invalid'},{password:''}])assert.throws(()=>validateInput({...input,...override}));
 for(const value of [{type:'eval',text:'x'},{type:'key',key:'F12'},{type:'click',x:-1,y:0},{type:'text',text:'x'.repeat(4097)},{type:'wheel',delta:99999}])assert.throws(()=>actionInput(value));
 assert.equal(actionInput({type:'click',x:10,y:10}).type,'click');
});
