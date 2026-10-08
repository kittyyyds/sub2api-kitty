import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
const require=createRequire(import.meta.url),ts=require('typescript');
const task={id:'fixture-task',kittyKeyId:'11'};
function moduleFor(fetch){
 const exports={};
 const modules={
  '@/lib/kitty':{assertKittySession:()=>({user:{id:1},keys:[{id:11}]}),kittyToken:()=>'fixture-session'},
  '@/services/file-storage':{uploadMediaFile:()=>{throw Error('unexpected upload')}},
  '@/lib/video-download':{}
 };
 const code=ts.transpileModule(readFileSync(new URL('../src/services/api/kitty-video.ts',import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText;
 vm.runInNewContext(code,{exports,require:id=>modules[id]??require(id),URL,AbortController,setTimeout,clearTimeout,Blob,fetch});
 return exports;
}
test('Canvas downloads the result directly with no host credentials or reference upload',async()=>{
 const requests=[];
 const mod=moduleFor(async(url,options)=>{requests.push({url,options});return requests.length===1?new Response(JSON.stringify({status:'completed',delivery_mode:'upstream_direct',url:'https://cdn.example/file.mp4?fixture=1'})):new Response(new Uint8Array([1,2,3]),{headers:{'content-type':'video/mp4','content-length':'3'}})});
 const blob=await mod.downloadKittyVideoBlob(task,new AbortController().signal,()=>{});
 assert.equal(blob.size,3);
 assert.equal(requests[0].options.headers.Authorization,'Bearer fixture-session');
 assert.deepEqual(JSON.parse(JSON.stringify(requests[1].options.headers)),{});
 assert.equal(requests[1].options.credentials,'omit');
 assert.equal(requests[1].options.referrerPolicy,'no-referrer');
});
test('Canvas CORS failure has no same-site content fallback',async()=>{
 const requests=[];
 const mod=moduleFor(async(url)=>{requests.push(url);if(requests.length===1)return new Response(JSON.stringify({status:'completed',delivery_mode:'upstream_direct',url:'https://cdn.example/file.mp4'}));throw new TypeError('CORS')});
 await assert.rejects(mod.downloadKittyVideoBlob(task,new AbortController().signal,()=>{}),/CORS/);
 assert.equal(requests.length,2);assert.equal(requests.some(u=>u.endsWith('/content')),false);
});
test('Canvas will not download uncertain, failed or expired unified tasks',async()=>{
 for(const data of [{status:'submission_unknown',delivery_mode:'upstream_direct'},{status:'failed',delivery_mode:'upstream_direct'},{status:'completed',delivery_mode:'upstream_direct',delivery_status:'expired'}]){
  let count=0;const mod=moduleFor(async()=>{count++;return new Response(JSON.stringify(data))});
  await assert.rejects(mod.downloadKittyVideoBlob(task,new AbortController().signal,()=>{}));assert.equal(count,1);
 }
});
