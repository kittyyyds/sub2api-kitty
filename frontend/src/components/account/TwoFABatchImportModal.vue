<script setup lang="ts">
import { computed, ref } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import OpenAITwoFAImport from './OpenAITwoFAImport.vue'
import OpenAIRequestTimezoneField from './OpenAIRequestTimezoneField.vue'
import { adminAPI } from '@/api/admin'
import { createTokenGuardV2Account } from '@/api/admin/accountTokenGuardV2'
import type { TokenGuardReloginAccount } from '@/api/admin/accountTokenGuard'
import { readOpenAIRequestTimezone, withOpenAIRequestTimezone } from '@/utils/openAIRequestTimezone'
import type { Group, Proxy } from '@/types'

const props = defineProps<{ proxies: Proxy[]; groups: Group[] }>()
const emit = defineEmits<{ close: []; created: [] }>()
const proxyId=ref<number|null>(null),groupIds=ref<number[]>([]),ws=ref('off'),fingerprint=ref('off'),syncModels=ref(true)
const timezone=ref(readOpenAIRequestTimezone()),busy=ref(false),locked=ref(false),warning=ref('')
const activeProxies=computed(()=>props.proxies.filter(p=>p.status==='active'&&(!p.expires_at||Date.parse(p.expires_at)>Date.now())))
const eligibleGroups=computed(()=>props.groups.filter(g=>g.status==='active'&&['openai','composite'].includes(g.platform)))
type Snapshot={proxy:number|null;groups:number[];extra:Record<string,unknown>;sync:boolean}
let batch:Snapshot|null=null
const createdInBatch=new Map<string,number>()
function prepare(){
 if(timezone.value.enabled&&timezone.value.mode==='proxy'&&!proxyId.value)throw new Error('select_proxy')
 batch={proxy:proxyId.value,groups:[...groupIds.value],sync:syncModels.value,extra:withOpenAIRequestTimezone({
  openai_oauth_responses_websockets_v2_mode:ws.value,openai_oauth_responses_websockets_v2_enabled:ws.value!=='off',
  ...(fingerprint.value==='off'?{}:{codex_fingerprint_mode:fingerprint.value})
 },{...timezone.value})}
 locked.value=true
}
function reset(){locked.value=false;batch=null;createdInBatch.clear();warning.value=''}
async function importCredential(credential:Record<string,unknown>,email:string,login:TokenGuardReloginAccount):Promise<'created'|'skipped'>{
 if(!batch)throw new Error('batch_not_prepared')
 let accountId=createdInBatch.get(email)
 if(!accountId){
  const result=await adminAPI.accounts.importCodexSession({content:JSON.stringify(credential),name:email,proxy_id:batch.proxy,group_ids:batch.groups,skip_default_group_bind:true,concurrency:1,priority:50,rate_multiplier:1,extra:batch.extra,skip_existing:true,update_existing:false})
  const item=result.items?.find(i=>i.action==='created'&&i.account_id)
  if(!item?.account_id){if(result.skipped>0&&result.failed===0)return 'skipped';throw new Error('account_import_failed')}
  accountId=item.account_id;createdInBatch.set(email,accountId);emit('created')
 }
 await createTokenGuardV2Account({account_id:accountId,login_email:login.email,credential_mode:'password_totp',engine:batch.proxy?'local_worker':undefined,proxy_source:'account',password:login.password,totp_secret:login.mfa_secret,enabled:true,auto_relogin_enabled:true})
 if(batch.sync){
  const catalog=await adminAPI.accounts.syncUpstreamModels(accountId)
  if(!catalog.models.length)throw new Error('empty_upstream_models')
  const account=await adminAPI.accounts.getById(accountId)
  await adminAPI.accounts.update(accountId,{credentials:{...account.credentials,model_mapping:Object.fromEntries(catalog.models.map(m=>[m,m]))}})
  if(catalog.warnings?.length)warning.value='账号及模型已保存，但部分模型元数据不完整，可在账号中重新同步。'
 }
 emit('created');return 'created'
}
</script>
<template>
 <BaseDialog :show="true" title="2FA账号添加 / 批量导入" width="wide" @close="!busy && emit('close')">
  <div class="space-y-5">
   <p class="rounded-lg bg-primary-50 p-3 text-sm text-primary-800 dark:bg-primary-950 dark:text-primary-200">沿用后台密码 + 2FA登录。选中代理后，首次授权、密码验证、2FA及换取凭证都使用该代理；失败不会改为直连。官方额外验证仍可能需要人工处理。</p>
   <fieldset :disabled="locked||busy" class="space-y-4">
    <div class="grid gap-4 md:grid-cols-2">
     <label><span class="input-label">住宅IP / 代理</span><select v-model="proxyId" class="input"><option :value="null">不使用账号代理（原登录服务网络）</option><option v-for="p in activeProxies" :key="p.id" :value="p.id">{{p.name}} · {{[p.country,p.city].filter(Boolean).join(' / ')}}</option></select><span class="input-hint">使用代理管理中已添加的住宅代理；名称不代表实际网络类型。</span></label>
     <label><span class="input-label">WS模式</span><select v-model="ws" class="input"><option value="off">关闭</option><option value="ctx_pool">上下文池</option><option value="passthrough">透传</option><option value="http_bridge">HTTP桥接</option></select></label>
     <label><span class="input-label">Codex指纹收敛</span><select v-model="fingerprint" class="input"><option value="off">关闭</option><option value="device">设备</option><option value="session">会话</option><option value="full">设备 + 会话</option></select></label>
     <label class="flex items-center gap-2"><input v-model="syncModels" type="checkbox" />导入后同步上游支持模型并保存模型白名单</label>
    </div>
    <OpenAIRequestTimezoneField v-model="timezone" :proxy-id="proxyId" />
    <div><span class="input-label">绑定分组（可多选）</span><div class="flex max-h-40 flex-wrap gap-3 overflow-y-auto rounded-lg border p-3 dark:border-dark-600"><label v-for="g in eligibleGroups" :key="g.id" class="flex gap-2 text-sm"><input v-model="groupIds" type="checkbox" :value="g.id" />{{g.name}}</label><span v-if="!eligibleGroups.length" class="text-sm text-gray-500">暂无可绑定的OpenAI或复合分组</span></div><p class="input-hint">未选择则不绑定分组。已有账号只跳过，不覆盖其代理、分组或凭据。</p></div>
   </fieldset>
   <p v-if="locked" class="text-xs text-amber-600">本批设置已锁定；失败重试沿用同一代理与设置。需要修改请点击下方重新填写。</p>
   <p v-if="warning" role="status" class="text-sm text-amber-600">{{warning}}</p>
   <OpenAITwoFAImport :proxy-id="proxyId" :login-environment="{enabled:timezone.enabled, mode:timezone.mode || 'manual', timezone:timezone.timezone}" :prepare="prepare" :import-credential="importCredential" @busy="busy=$event" @reset="reset" />
  </div>
 </BaseDialog>
</template>
