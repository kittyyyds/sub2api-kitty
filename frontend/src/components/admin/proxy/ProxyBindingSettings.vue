<script setup lang="ts">
import { ref,onMounted } from 'vue'
import { getProxyBindingPolicy,saveProxyBindingPolicy,type ProxyBindingPolicy } from '@/api/admin/proxyBinding'
const policy=ref<ProxyBindingPolicy|null>(null),busy=ref(false),error=ref(''),notice=ref('')
async function load(){busy.value=true;error.value='';try{policy.value=await getProxyBindingPolicy()}catch{error.value='读取规则失败，请重试'}finally{busy.value=false}}
async function save(){if(!policy.value)return;busy.value=true;error.value='';notice.value='';try{policy.value=await saveProxyBindingPolicy(policy.value);notice.value='已保存。仅限制后续新增绑定，已有账号继续使用原代理。'}catch(e){error.value=(e as Error).message||'保存失败'}finally{busy.value=false}}
onMounted(()=>void load())
</script>
<template>
 <section class="mx-auto max-w-3xl space-y-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
  <h2 class="text-lg font-semibold">账号 IP 分配与历史绑定规则</h2>
  <p v-if="error" role="alert" class="text-red-600">{{error}} <button class="underline" @click="load">重试</button></p>
  <p v-if="notice" role="status" class="text-emerald-600">{{notice}}</p>
  <form v-if="policy" class="space-y-5" @submit.prevent="save"><fieldset :disabled="busy" class="space-y-5">
   <label class="flex gap-2"><input v-model="policy.enabled" type="checkbox" />启用 IP 绑定规则</label>
   <label class="flex gap-2"><input v-model="policy.only_twofa" type="checkbox" />仅对通过 2FA 导入的 OAuth 账号执行</label>
   <fieldset><legend class="mb-2 text-sm font-medium">参与规则的 OAuth 平台</legend><div class="flex flex-wrap gap-4"><label v-for="p in ['openai','anthropic','gemini','antigravity','grok']" :key="p" class="flex gap-2"><input v-model="policy.platforms" :value="p" type="checkbox" />{{p}}</label></div></fieldset>
   <label class="block text-sm">每个 IP 历史绑定账号上限<input v-model.number="policy.history_limit" class="input mt-2 max-w-xs" type="number" min="1" max="10000" required /></label>
   <label class="flex gap-2"><input v-model="policy.auto_assign" type="checkbox" />导入前自动分配空闲 IP</label>
   <p class="rounded-lg bg-blue-50 p-3 text-sm leading-6 text-blue-800 dark:bg-blue-950/30 dark:text-blue-200">自动分配选择历史未满、当前没有任何账号绑定、未被导入任务预留的有效 IP。每个账号单独分配，登录和保存使用同一代理。无可用 IP 时明确报错，不直连。</p>
   <p class="text-sm leading-6 text-gray-500">历史只统计所选范围内的不同账号；解绑、删除不退还名额，重登不重复累计。满额只停止规则内新增绑定，不中断已有账号。手动确认每次只额外放行 1 个新账号，自动分配不会消费这个名额。</p>
   <p class="text-xs leading-5 text-gray-500">历史从本功能启用记录及已有可验证绑定开始。没有可靠来源的旧账号不会猜测为 2FA。代理出口检测会合并同一出口 IP 的记录；请使用固定出口代理。</p>
   <button class="btn btn-primary" :disabled="busy" type="submit">{{busy?'保存中…':'保存 IP 规则'}}</button>
  </fieldset></form>
 </section>
</template>
