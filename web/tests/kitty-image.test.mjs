import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import { webcrypto } from 'node:crypto';
const require=createRequire(import.meta.url),ts=require('typescript');
const code=ts.transpileModule(readFileSync(new URL('../src/services/api/kitty-image.ts',import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText;
function fixture(responses){
 const exports={},requests=[];
 vm.runInNewContext(code,{exports,require:()=>({kittyHeaders:()=>({Authorization:'Bearer session','X-Canvas-Key-ID':'7'}),kittyToken:()=> 'session'}),crypto:webcrypto,AbortSignal,Date,setTimeout:f=>f(),fetch:async(url,options)=>{requests.push({url,options});const r=responses.shift();if(r instanceof Error)throw r;return {ok:r.status<400,status:r.status,json:async()=>r.data}}});
 return {...exports,requests};
}
test('async image submits once and polls with original selected key',async()=>{
 const id='simg_12345678-1234-1234-1234-123456789abc';
 const f=fixture([{status:202,data:{id,status:'processing'}},{status:200,data:{id,status:'processing'}},{status:200,data:{id,status:'completed',result:{data:[{url:'https://cdn.example/image.png'}]}}}]);
 const result=await f.requestKittyImage({model:'Banana-2-香蕉2'},'test',['data:image/png;base64,fixture'],'2048x1152');
 assert.equal(result.data.length,1);assert.equal(f.requests.filter(r=>r.options.method==='POST').length,1);
 assert.equal(JSON.parse(f.requests[0].options.body).n,1);assert.equal(f.requests[0].options.headers.Prefer,'respond-async');
 assert.ok(f.requests.every(r=>r.url.startsWith('/canvas-api/')&&r.options.headers['X-Canvas-Key-ID']==='7'));
});
test('unknown paid submission is never retried',async()=>{
 const f=fixture([new Error('network failed')]);
 await assert.rejects(f.requestKittyImage({model:'image'},'test',[]));assert.equal(f.requests.length,1);
});
test('4K selection requests a 4096 long edge',()=>{
 const f=fixture([]);assert.equal(f.kittyImageSize('4k','1:1'),'4096x4096');assert.equal(f.kittyImageSize('high','16:9'),'4096x2304');
});
