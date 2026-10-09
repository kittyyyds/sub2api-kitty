const fs=require('node:fs');const path=require('node:path');const http=require('node:http');const assert=require('node:assert/strict');
const {chromium}=require('D:/ReverseTools/js/node_modules/playwright');
const dist='D:/AI-Storage/Codex/Artifacts/kitty-image-backend-20261009-r2/kittyapi-tree/backend/internal/web/dist';
const output='D:/AI-Storage/Reviews/kitty-image-upstream-20261009-r2/ui-evidence';fs.mkdirSync(output,{recursive:true});
const writes=[],requests=[],errors=[];
let video={auto_sync:true,auto_import_account_models:true,follow_increase:true,follow_decrease:false,interval_seconds:60,markup:{output_per_second:.1,reference_per_second:.1,per_request:.5},reference_retention_hours:24,max_concurrent_tasks:3,reference_user_mb:512,reference_total_mb:2048};
let settings={auto_sync:false,auto_import_account_models:false,follow_increase:true,follow_decrease:false,interval_seconds:60,markup:0};
const catalog={version:'synthetic-ui',last_attempt:'2026-10-09T00:00:00Z',last_success:'2026-10-09T00:00:00Z',last_error:'',models:{'Banana-2-香蕉2':{model:'Banana-2-香蕉2',unit_price:.2,billing_mode:'per_request',description:'合成测试说明'}}};
const groups=[{id:70001,name:'合成图片分组',pricing:[{models:['Banana-2-香蕉2'],image:{provider:'suanli',upstream_model:'Banana-2-香蕉2',auto_price:true,cost:.2,applied_markup:0,description:'合成测试说明',applied_prices:{'1K':.2}}}],prices:{'banana-2-香蕉2':{'1K':.2,'2K':.2,'4K':.2}}}];
const user={id:70000,email:'fixture@example.invalid',username:'验收管理员',role:'super_admin',status:'active',balance:100,concurrency:5};
const server=http.createServer((req,res)=>{
 const p=decodeURIComponent(new URL(req.url,'http://localhost').pathname);let file=path.resolve(dist,'.'+p);
 if(!file.startsWith(path.resolve(dist)+path.sep)||!fs.existsSync(file)||!fs.statSync(file).isFile())file=path.join(dist,'index.html');
 const mime={'.html':'text/html','.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.woff2':'font/woff2'}[path.extname(file)]||'application/octet-stream';res.writeHead(200,{'Content-Type':mime});fs.createReadStream(file).pipe(res);
});
(async()=>{
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const base='http://127.0.0.1:'+server.address().port;
 const browser=await chromium.launch({executablePath:'D:/ReverseTools/browsers/chromium-1243/chrome-win64/chrome.exe',headless:true});
 try{
 const context=await browser.newContext({viewport:{width:1536,height:1000}});
 await context.addInitScript(u=>{localStorage.setItem('auth_token','synthetic-ui-only');localStorage.setItem('auth_user',JSON.stringify(u));localStorage.setItem('locale','zh');localStorage.setItem('admin_guide_'+u.id+'_'+u.role+'_v4_interactive','true');},user);
 await context.route('**/*',async route=>{
  const req=route.request(),url=new URL(req.url());if(url.origin!==base){return route.abort();}
  if(!url.pathname.startsWith('/api/'))return route.continue();
  const p=url.pathname,method=req.method();requests.push({p,method});let data={};
  if(method!=='GET')writes.push({p,method,body:req.postDataJSON()});
  if(p==='/api/v1/auth/me')data=user;
  else if(p==='/api/v1/admin/compliance')data={required:false,version:'synthetic'};
  else if(p==='/api/v1/settings/public')data={site_name:'Kitty 合成验收',registration_enabled:true,run_mode:'standard',version:'2.9.9-kitty.14',custom_menu_items:[],payment_enabled:true};
  else if(p==='/api/v1/admin/image-upstream'){if(method==='PUT')settings=req.postDataJSON();data={settings,catalog,groups};}
  else if(p==='/api/v1/admin/image-upstream/sync')data=catalog;
  else if(p==='/api/v1/admin/video-upstream'){if(method==='PUT')video=req.postDataJSON();data={settings:video,catalog:{...catalog,models:{},changes:[]}};}
  else if(p==='/api/v1/admin/groups')data={items:[],total:0,page:1,page_size:100};
  else if(p.includes('/announcements'))data={items:[],total:0};
  else if(p.includes('/plugins'))data=[];
  return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({code:0,message:'success',data})});
 });
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
 await page.goto(base+'/admin/video-upstream',{waitUntil:'networkidle'});
 await page.getByRole('tab',{name:'图片跟价',exact:true}).click();
 const panel=page.getByTestId('image-upstream-panel');await panel.getByLabel('图片每次加价').fill('0.17');
 await page.getByRole('tab',{name:'视频跟价',exact:true}).click();await page.getByRole('tab',{name:'图片跟价',exact:true}).click();
 assert.equal(await panel.getByLabel('图片每次加价').inputValue(),'0.17');
 await panel.getByRole('button',{name:'保存图片设置',exact:true}).click();await panel.getByRole('status').waitFor();
 assert.equal(writes.length,1);assert.equal(writes[0].p,'/api/v1/admin/image-upstream');assert.equal(writes[0].body.markup,.17);assert.equal(video.markup.per_request,.5);
 await panel.getByRole('button',{name:'只读取上游价格',exact:true}).click();await page.getByText('上游价格已读取，没有修改客户售价。',{exact:true}).waitFor();
 assert.equal(writes.length,2);assert.equal(writes[1].p,'/api/v1/admin/image-upstream/sync');
 await page.screenshot({path:path.join(output,'desktop-image.png'),fullPage:true});
 await page.setViewportSize({width:390,height:844});await page.reload({waitUntil:'networkidle'});await page.getByRole('tab',{name:'图片跟价',exact:true}).click();await page.screenshot({path:path.join(output,'mobile-image.png'),fullPage:true});
 const dimensions=await page.evaluate(()=>({viewport:innerWidth,document:document.documentElement.scrollWidth}));
 assert.ok(dimensions.document<=dimensions.viewport+2,'Whole page overflows on mobile');
 assert.deepEqual(errors,[]);
 fs.writeFileSync(path.join(output,'result.json'),JSON.stringify({passed:true,independent_save:true,draft_survives_tab_switch:true,readonly_price_lookup:true,dimensions,requests,writes,errors},null,2));
 console.log(JSON.stringify({passed:true,independent_save:true,mobile_overflow:false}));
 }catch(e){fs.writeFileSync(path.join(output,'failure.json'),JSON.stringify({message:e.message,requests,writes,errors},null,2));throw e;}finally{await browser.close();server.close();}
})().catch(e=>{console.error(e);server.close();process.exitCode=1;});
