import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from '../server.mjs';
test('private API rejects anonymous calls, caps sessions, and cleans canceled jobs',async()=>{
 const token='fixture-token-'.repeat(4);let closed=0,started=0;
 const server=createServer({token,maxSessions:1,sessionFactory:()=>({id:'a'.repeat(48),started:Date.now(),async start(){started++;},async close(){closed++;},progress(){return {status:'waiting_interaction',stage:'security_verification'};}})});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const origin=`http://127.0.0.1:${server.address().port}`;
 const call=(path,method='GET',data,auth=token)=>fetch(origin+path,{method,headers:{authorization:`Bearer ${auth}`,'content-type':'application/json'},body:data?JSON.stringify(data):undefined});
 try{
  assert.equal((await call('/health','GET',null,'wrong')).status,401);
  assert.equal((await call('/health')).status,200);
  const input={email:'a@example.invalid',password:'fixture',totp:'fixture',proxy:'http://127.0.0.1:9',timezone:'UTC'};
  assert.equal((await call('/sessions','POST',{...input,timezone:'bad'})).status,400);assert.equal(started,0);
  assert.equal((await call('/sessions','POST',input)).status,202);
  assert.equal((await call('/sessions','POST',input)).status,429);
  assert.equal((await call('/sessions/'+'b'.repeat(48))).status,404);
  assert.equal((await call('/sessions/'+'a'.repeat(48),'DELETE')).status,200);assert.equal(closed,1);
  assert.equal((await call('/health')).status,200);
 }finally{server.closeAllConnections();await new Promise(r=>server.close(r));}
});
