<script setup lang="ts">
import { computed } from 'vue'
import type { OpenAITwoFAAdmissionConfig as IQBootstrap } from '@/utils/openaiTwoFAAdmission'
import type { Group } from '@/types'
const props=defineProps<{modelValue:IQBootstrap;groups:Group[]}>()
const emit=defineEmits<{ 'update:modelValue':[IQBootstrap] }>()
function set<K extends keyof IQBootstrap>(key:K,value:IQBootstrap[K]) {emit('update:modelValue',{...props.modelValue,[key]:value})}
const validTarget=computed(()=>props.groups.some(g=>g.id===props.modelValue.isolation_group_id && g.platform==='openai' && g.status==='active'))
function selectGroup(id:number) {
 emit('update:modelValue',{...props.modelValue,isolation_group_id:id})
}
</script>
<template>
 <fieldset class="space-y-3 rounded-xl border border-blue-200 p-4 dark:border-blue-900" data-testid="iq-bootstrap-settings">
  <legend class="px-1 text-sm font-semibold">导入后自动糖果验证</legend>
  <label class="flex items-center gap-2 text-sm"><input data-testid="admission-enabled" type="checkbox" :checked="modelValue.enabled" @change="set('enabled',($event.target as HTMLInputElement).checked)" />新 OpenAI 2FA 账号先验证再开放调度</label>
  <p class="text-xs leading-5 text-gray-500">采用新版 2FA 准入规则，独立于上方首次配置开关和平台选择。仅处理服务端确认的首次 OpenAI 2FA OAuth 导入：固定 1 并发并关闭客户调度，后台测试继续；重登和重复导入不重新初始化。</p>
  <div v-if="modelValue.enabled" class="space-y-3">
   <div class="grid gap-3 sm:grid-cols-2">
    <label class="text-sm">测试模型<input class="input mt-1 w-full" data-testid="admission-model" :value="modelValue.model_id" required maxlength="100" @input="set('model_id',($event.target as HTMLInputElement).value)" /></label>
    <label class="text-sm">思考等级<select class="input mt-1 w-full" data-testid="admission-effort" :value="modelValue.reasoning_effort" @change="set('reasoning_effort',($event.target as HTMLSelectElement).value)"><option value="none">无（none）</option><option value="low">低（low）</option><option value="minimal">最低（minimal，模型需支持）</option><option value="medium">中</option><option value="high">高</option><option value="xhigh">很高（xhigh）</option><option value="max">最大（max）</option><option value="ultra">极高（ultra）</option></select></label>
    <label class="text-sm">完成后等待（秒）<input class="input mt-1 w-full" type="number" min="1" max="86400" data-testid="admission-wait_seconds" :value="modelValue.wait_seconds" @input="set('wait_seconds',Number(($event.target as HTMLInputElement).value))" /></label>
    <label class="text-sm">连续答对 21 次数<input class="input mt-1 w-full" type="number" min="1" max="1000" data-testid="admission-pass_count" :value="modelValue.pass_count" @input="set('pass_count',Number(($event.target as HTMLInputElement).value))" /></label>
    <label class="text-sm">通过后并发<input class="input mt-1 w-full" type="number" min="1" max="10000" data-testid="admission-passed_concurrency" :value="modelValue.passed_concurrency" @input="set('passed_concurrency',Number(($event.target as HTMLInputElement).value))" /></label>
    <label class="text-sm">连续回答 29 隔离次数<input data-testid="admission-isolation-count" class="input mt-1 w-full" type="number" min="1" max="1000" :value="modelValue.isolation_count" @input="set('isolation_count',Number(($event.target as HTMLInputElement).value))" /></label>
   </div>
   <p class="rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950/40 dark:text-blue-200">连续 {{modelValue.pass_count}} 次完整答对 21 后，自动升至 {{modelValue.passed_concurrency}} 并发并打开调度。普通并发升档从通过后开始计客户成功请求。</p>
   <label class="block text-sm">隔离目标分组<select data-testid="admission-group" class="input mt-1 w-full" :value="modelValue.isolation_group_id" @change="selectGroup(Number(($event.target as HTMLSelectElement).value))"><option :value="0">请选择启用中的 OpenAI 分组（启用验证时必选）</option><option v-if="modelValue.isolation_group_id && !groups.some(g => g.id===modelValue.isolation_group_id)" :value="modelValue.isolation_group_id" disabled>原分组 #{{modelValue.isolation_group_id}} 已不可用，请重新选择</option><option v-for="g in groups" :key="g.id" :value="g.id" :disabled="g.platform!=='openai'||g.status!=='active'">{{g.name}} · #{{g.id}} · {{g.platform}} · {{g.status==='active'?'启用':'停用'}}</option></select></label>
   <p v-if="!validTarget" class="text-xs text-gray-500">读取已有全部分组。新版规则要求选择有效的 OpenAI 隔离分组后才能保存启用；不会自动新建分组。</p>
   <p class="text-xs leading-5 text-gray-500">连续 {{modelValue.isolation_count}} 次完整回答 29 后，移出全部原分组并加入所选隔离组，保留代理和当前并发、停止自动升档；测试继续，后续答对不会自动恢复。其他答案、异常或不完整输出打断连续计数。</p>
  </div>
  <p class="text-xs leading-5 text-gray-500">保存后仅对后续新账号生效。已有账号沿用导入时规则和原执行器，保留全部记录、进度和隔离状态；关闭此开关不会中断已创建的测试。</p>
 </fieldset>
</template>
