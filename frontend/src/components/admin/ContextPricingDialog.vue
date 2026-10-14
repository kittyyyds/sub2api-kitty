<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { contextPricing, type ContextRule } from '@/api/admin/contextPricing'
const props=defineProps<{show:boolean}>(),emit=defineEmits<{close:[]}>()
const platforms=[{value:'openai',label:'GPT'},{value:'anthropic',label:'Claude'},{value:'grok',label:'Grok'},{value:'gemini',label:'Gemini'},{value:'deepseek',label:'DeepSeek'},{value:'zhipu',label:'GLM'},{value:'kimi',label:'Kimi'},{value:'minimax',label:'MiniMax'},{value:'opencode-go',label:'OpenCode Go'},{value:'typesafe',label:'TypeSafe'},{value:'openai-compatible',label:'其他 / 全部已配置文本模型'}]
const fresh=():ContextRule=>({id:0,revision:0,platform:'openai',model:'',threshold_tokens:500000,input_multiplier:2,output_multiplier:2,cache_read_multiplier:2,cache_write_multiplier:2,enabled:false})
const rule=ref(fresh()),rules=ref<ContextRule[]>([]),models=ref<string[]>([]),error=ref(''),modelError=ref(''),busy=ref(false),loadingModels=ref(false),pending=ref(false)
const simulation=ref({input:500000,read:0,write:0})
const total=computed(()=>Math.max(0,Number(simulation.value.input)||0)+Math.max(0,Number(simulation.value.read)||0)+Math.max(0,Number(simulation.value.write)||0))
const applies=computed(()=>total.value>rule.value.threshold_tokens)
const enabledCount=computed(()=>rules.value.filter(r=>r.enabled).length)
const fields=[{key:'input_multiplier',label:'输入'},{key:'output_multiplier',label:'输出'},{key:'cache_read_multiplier',label:'缓存读'},{key:'cache_write_multiplier',label:'缓存写'}] as const
const thresholdK=computed({get:()=>rule.value.threshold_tokens/1000,set:(v:number)=>rule.value.threshold_tokens=Math.round(Number(v)*1000)})
const options=computed(()=>[...new Set([...models.value,rule.value.model].filter(Boolean))].map(value=>({value,label:value})))
let sequence=0
async function loadModels(){const id=++sequence;models.value=[];modelError.value='';loadingModels.value=true;try{const data=await contextPricing.models(rule.value.platform);if(id===sequence)models.value=data}catch{if(id===sequence)modelError.value='模型目录加载失败，可重试或填写精确计费模型ID。'}finally{if(id===sequence)loadingModels.value=false}}
async function load(){busy.value=true;error.value='';try{rules.value=await contextPricing.list()}catch{error.value='读取规则失败，不能据此判断没有配置。'}finally{busy.value=false}}
function edit(r?:ContextRule){rule.value=r?{...r}:fresh();pending.value=false}
async function save(){busy.value=true;error.value='';try{await contextPricing.save({...rule.value});pending.value=false;await load();edit()}catch(e){error.value=(e as Error).message||'保存失败，请刷新确认版本后重试'}finally{busy.value=false}}
async function remove(r:ContextRule){if(!window.confirm(`删除 ${r.model} 的全局规则？删除后恢复原有上下文计费。`))return;busy.value=true;try{await contextPricing.remove(r);await load();if(rule.value.id===r.id)edit()}catch{error.value='删除失败，可能已被其他管理员修改，请刷新。'}finally{busy.value=false}}
function simulate(n:number){simulation.value={input:n,read:0,write:0}}
watch(()=>props.show,v=>{if(v){void load();void loadModels()}},{immediate:true})
watch(()=>rule.value.platform,()=>void loadModels())
watch(rule,()=>pending.value=false,{deep:true})
</script>
<template>
 <BaseDialog :show="show" title="长上下文定价" width="extra-wide" @close="emit('close')">
  <div class="space-y-5">
   <div class="grid gap-3 sm:grid-cols-3"><div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600"><p class="text-xs text-gray-500">全局优先规则</p><p class="mt-1 text-2xl font-semibold">{{rules.length}} <span class="text-sm text-gray-400">条</span></p></div><div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600"><p class="text-xs text-gray-500">已启用</p><p class="mt-1 text-2xl font-semibold text-emerald-600">{{enabledCount}}</p></div><div class="rounded-xl bg-primary-50 p-4 dark:bg-primary-950"><p class="text-xs text-gray-500">优先级与回退</p><p class="mt-2 text-sm font-medium">全局模型规则 → 原版本上下文规则</p></div></div>
   <p class="text-sm text-gray-500">仅覆盖长上下文阶梯；基础单价、分组、Fast与思考等级倍率仍有效，不叠加旧阶梯。未设置、停用或删除即恢复原规则；不改上游成本统计。</p>
   <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950">{{error}}</p>
   <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
    <section class="min-w-0 space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
     <div class="flex items-center justify-between"><h4 class="font-semibold">{{rule.id?'编辑规则':'新增规则'}}</h4><button class="text-sm text-primary-600" :disabled="busy" @click="edit()">清空 / 新增</button></div>
     <div class="grid gap-4 sm:grid-cols-2"><label><span class="input-label">文本平台</span><Select v-model="rule.platform" :options="platforms" :disabled="busy||!!rule.id" /></label><label><span class="input-label">选择模型</span><Select v-model="rule.model" :options="options" searchable :loading="loadingModels" :disabled="busy||!!rule.id" /></label></div>
     <p v-if="modelError" class="text-xs text-amber-600">{{modelError}} <button class="underline" @click="loadModels">重试</button></p>
     <p v-else class="text-xs text-gray-500">自动合并本站账号模型、分组/渠道价卡及模型目录。不触发上游测试请求。</p>
     <label class="block"><span class="input-label">精确计费模型ID（也可手填）</span><input v-model.trim="rule.model" class="input font-mono" :disabled="busy||!!rule.id" placeholder="gpt-6.1-sol" /><span class="input-hint">按实际计费模型ID精确匹配，不支持通配符；平台仅用于筛选，同名模型全站生效。</span></label>
     <div class="grid gap-4 sm:grid-cols-2"><label><span class="input-label">超过上下文（K）</span><input v-model.number="thresholdK" class="input" type="number" min="0.001" max="100000" step="0.001" :disabled="busy" /></label><label class="flex items-center gap-2 pt-6"><input v-model="rule.enabled" type="checkbox" :disabled="busy" />启用此规则</label></div>
     <div class="grid grid-cols-2 gap-3"><label v-for="f in fields" :key="f.key"><span class="input-label">{{f.label}}扣费倍率</span><div class="flex items-center gap-2"><input v-model.number="rule[f.key]" type="number" min="0.01" max="100" step="0.01" class="input" :disabled="busy" /><span class="text-gray-400">×</span></div></label></div>
     <p class="text-xs text-gray-500">1K = 1000 tokens。仅严格超过门槛时整次请求按各项倍率计费，不是只乘超出部分。</p>
     <div v-if="pending" role="status" class="space-y-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200"><p>确认{{rule.id?'修改':'新增'}} {{rule.model}}：&gt;{{thresholdK}}K，输入/输出/缓存读/写 = {{rule.input_multiplier}} / {{rule.output_multiplier}} / {{rule.cache_read_multiplier}} / {{rule.cache_write_multiplier}}×。{{rule.enabled?'启用':'保持停用'}}。</p><p class="text-xs">影响保存后结算的请求；其他应用实例通常1秒内同步。数据库短时不可用时保留最近一次有效规则。</p><button class="btn btn-primary" :disabled="busy" @click="save">确认保存</button></div>
     <button v-else class="btn btn-primary w-full" :disabled="busy||!rule.model" @click="pending=true">保存规则</button>
    </section>
    <aside class="space-y-4 self-start rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800">
     <h4 class="font-semibold">门槛模拟 <span class="ml-1 text-xs font-normal text-gray-500">不发请求、不扣余额</span></h4>
     <p class="text-xs text-gray-500">上下文 = 输入 + 缓存读取 + 缓存写入，不包括输出。</p>
     <label class="block"><span class="input-label">输入 tokens</span><input v-model.number="simulation.input" class="input" type="number" min="0" /></label>
     <label class="block"><span class="input-label">缓存读取 tokens</span><input v-model.number="simulation.read" class="input" type="number" min="0" /></label>
     <label class="block"><span class="input-label">缓存写入 tokens</span><input v-model.number="simulation.write" class="input" type="number" min="0" /></label>
     <div class="flex flex-wrap gap-2"><button v-for="n in [272000,rule.threshold_tokens,rule.threshold_tokens+1]" :key="n" class="rounded-md border px-2 py-1 text-xs dark:border-dark-600" @click="simulate(n)">{{n.toLocaleString()}}</button></div>
     <div class="rounded-xl p-4" :class="applies?'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200':'bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-200'"><p class="text-2xl font-semibold">{{total.toLocaleString()}}</p><p class="mt-1 text-sm">{{applies?'超过门槛':'未超过门槛（含等于）'}}</p><p class="mt-2 text-xs">{{!rule.enabled?'草稿未启用，保存后仍沿用原计费。':applies?'整次请求使用以下倍率':'全局规则生效，各项上下文倍率为1，不叠加旧阶梯'}}</p><div class="mt-3 grid grid-cols-2 gap-2 text-xs"><span v-for="f in fields" :key="f.key">{{f.label}} ×{{applies?rule[f.key]:1}}</span></div></div>
    </aside>
   </div>
   <section class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600"><div class="flex justify-between p-4"><h4 class="font-semibold">已保存规则</h4><button class="text-sm text-primary-600" :disabled="busy" @click="load">刷新</button></div><div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800"><tr><th class="p-3">模型</th><th>上下文门槛</th><th>输入 / 输出 / 缓存读 / 写</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="r in rules" :key="r.id" class="border-t border-gray-100 dark:border-dark-700"><td class="p-3 font-mono">{{r.model}}</td><td>&gt; {{r.threshold_tokens/1000}}K</td><td>{{r.input_multiplier}} / {{r.output_multiplier}} / {{r.cache_read_multiplier}} / {{r.cache_write_multiplier}}</td><td><span :class="r.enabled?'text-emerald-600':'text-gray-400'">{{r.enabled?'启用':'停用'}}</span></td><td class="space-x-2 whitespace-nowrap pr-3"><button class="text-primary-600" :disabled="busy" @click="edit(r)">编辑 / 启停</button><button class="text-red-600" :disabled="busy" @click="remove(r)">删除</button></td></tr></tbody></table></div><p v-if="!rules.length&&!busy&&!error" class="p-6 text-center text-sm text-gray-500">暂无规则，保持原版本计费逻辑。</p></section>
  </div>
 </BaseDialog>
</template>
