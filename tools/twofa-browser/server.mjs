import http from 'node:http';
import crypto from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {validateInput} from './core.mjs';
import {LoginSession} from './session.mjs';

export function createServer({token,sessionFactory=data=>new LoginSession(data),maxSessions=2}={}){
 if(typeof token!=='string'||token.length<32)throw new Error('A private worker token of at least 32 characters is required');
 const sessions=new Map();
 async function body(req){let raw='';for await(const chunk of req){raw+=chunk;if(Buffer.byteLength(raw)>16384)throw new Error('too_large');}return JSON.parse(raw||'{}');}
 const send=(res,code,value)=>{res.writeHead(code,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(value));};
 const server=http.createServer(async(req,res)=>{
  const provided=Buffer.from(req.headers.authorization||''),expected=Buffer.from('Bearer '+token);
  if(provided.length!==expected.length||!crypto.timingSafeEqual(provided,expected))return send(res,401,{code:'unauthorized'});
  try{
   const path=new URL(req.url,'http://worker').pathname;
   if(req.method==='GET'&&path==='/health')return send(res,200,{ready:true,active:sessions.size});
   if(req.method==='POST'&&path==='/sessions'){
    if(sessions.size>=maxSessions)return send(res,429,{code:'browser_busy'});
    const data=validateInput(await body(req));
    // Recheck after the asynchronous body read to avoid overbooking.
    if(sessions.size>=maxSessions)return send(res,429,{code:'browser_busy'});
    const session=sessionFactory(data);sessions.set(session.id,session);
    void session.start();return send(res,202,{id:session.id});
   }
   const match=/^\/sessions\/([a-f0-9]{48})(?:\/(frame|input|resume))?$/.exec(path);
   const session=match&&sessions.get(match[1]);if(!session)return send(res,404,{code:'not_found'});
   if(req.method==='DELETE'&&!match[2]){sessions.delete(session.id);await session.close();return send(res,200,{deleted:true});}
   if(req.method==='GET'&&!match[2])return send(res,200,session.progress());
   if(req.method==='GET'&&match[2]==='frame'){const frame=await session.frame();res.writeHead(200,{'Content-Type':'image/jpeg','Cache-Control':'no-store, private','X-Content-Type-Options':'nosniff'});return res.end(frame);}
   if(req.method==='POST'&&match[2]==='input'){await session.input(await body(req));return send(res,200,{accepted:true});}
   if(req.method==='POST'&&match[2]==='resume'){session.resume();return send(res,200,{accepted:true});}
   return send(res,405,{code:'method_not_allowed'});
  }catch(e){return send(res,e.message==='not_ready'||e.message==='busy'?409:400,{code:'request_failed'});}
 });
 const gc=setInterval(()=>{for(const [id,s] of sessions)if(Date.now()-s.started>25*60*1000){sessions.delete(id);void s.close();}},15000);gc.unref();
 server.on('close',()=>{clearInterval(gc);for(const s of sessions.values())void s.close();sessions.clear();});
 return server;
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href){
 const server=createServer({token:process.env.KITTY_LOGIN_BROWSER_TOKEN});
 server.listen(Number(process.env.PORT||8080),'0.0.0.0',()=>console.log('Two-factor login browser ready'));
 for(const sig of ['SIGTERM','SIGINT'])process.on(sig,()=>server.close(()=>process.exit(0)));
}
