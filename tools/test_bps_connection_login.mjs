import {test} from 'node:test';
import assert from 'node:assert/strict';
import {validateAuthorization,validateCallback,loginBPSConnection} from '../backend/internal/reauthruntime/bps_connection_login.mjs';
const redirect='https://bps.openai.com/basispoints/extension/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3/auth/callback';
const auth=()=>new URL('https://auth.openai.com/api/accounts/authorize?'+new URLSearchParams({client_id:'app_fnr0pYvVwwFDocDumLG3H2Bp',redirect_uri:redirect,response_type:'code',code_challenge_method:'S256',code_challenge:'server-challenge',state:'server-state',nonce:'server-nonce'}));
test('authorization URL is pinned to BPS with server PKCE and nonce',()=>{
 assert.equal(validateAuthorization(auth().toString()).origin,'https://auth.openai.com');
 for(const [key,value] of [['client_id','codex'],['redirect_uri','https://example.invalid/callback'],['code_challenge_method','plain'],['nonce',''],['state','']]){const u=auth();u.searchParams.set(key,value);assert.throws(()=>validateAuthorization(u.toString()));}
 const duplicate=auth();duplicate.searchParams.append('state','other');assert.throws(()=>validateAuthorization(duplicate.toString()));
 assert.throws(()=>validateAuthorization(auth().toString().replace('auth.openai.com','example.invalid')));
});
test('callbacks reject state, origin, duplicate code and error responses',()=>{
 const good=redirect+'?state=server-state&code=synthetic';assert.equal(validateCallback(good,'server-state'),good);
 for(const value of [good+'&code=second',good+'#fragment',good+'&error=denied',good.replace('server-state','wrong'),good.replace('bps.openai.com','example.invalid')])assert.throws(()=>validateCallback(value,'server-state'));
});
test('password/TOTP flow returns callback without exchanging tokens',async()=>{
 process.env.KITTY_BPS_AUTH_URL=auth().toString();
 const calls=[];const client={jar:{value:()=>''},follow:async()=>({finalUrl:'https://auth.openai.com/log-in/password'})};
 const helpers={pickWorkspaceId:()=>null,isPasswordLoginPage:()=>true,verifyPassword:async(_c,o)=>{calls.push('password');assert.equal(o.password,'synthetic');return {};},completeTotpMfaIfNeeded:async()=>{calls.push('totp');return {};},selectChatgptLoginWorkspaceIfNeeded:async()=>({}),continueFlow:async()=>({finalUrl:redirect+'?state=server-state&code=synthetic'}),getContinueUrl:()=>''};
 await loginBPSConnection(client,{authBase:'https://auth.openai.com',email:'fixture@example.invalid',password:'synthetic',totpSecret:'JBSWY3DPEHPK3PXP'},helpers);
 assert.deepEqual(calls,['password','totp']);assert.equal(client.bpsConnection.codeVerifier,'server-owned');assert.equal(client.bpsConnection.state,'server-state');
 delete process.env.KITTY_BPS_AUTH_URL;
});
test('additional verification stops instead of pretending authorization succeeded',async()=>{
 process.env.KITTY_BPS_AUTH_URL=auth().toString();
 const client={jar:{value:()=>''},follow:async()=>({finalUrl:'https://auth.openai.com/email-verification'})};
 await assert.rejects(loginBPSConnection(client,{authBase:'https://auth.openai.com'},{isPasswordLoginPage:()=>false,isAuthLoginPage:()=>false}),/BPS_INTERACTION_REQUIRED/);
 assert.equal(client.bpsConnection,undefined);delete process.env.KITTY_BPS_AUTH_URL;
});
