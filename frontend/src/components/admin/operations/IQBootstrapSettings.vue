<script setup lang="ts">
import { computed } from 'vue'
import type { IQBootstrap } from '@/api/admin/autoConfig'
import type { Group } from '@/types'
const props=defineProps<{modelValue:IQBootstrap;groups:Group[];platform:string}>()
const emit=defineEmits<{ 'update:modelValue':[IQBootstrap] }>()
function set<K extends keyof IQBootstrap>(key:K,value:IQBootstrap[K]) {emit('update:modelValue',{...props.modelValue,[key]:value})}
const validTarget=computed(()=>props.groups.some(g=>g.id===props.modelValue.quarantine_group_id && g.platform==='openai' && g.status==='active'))
function selectGroup(id:number) {
 emit('update:modelValue',{...props.modelValue,quarantine_group_id:id,quarantine_enabled:id>0 && props.modelValue.quarantine_enabled})
}
</script>
<template>
 <fieldset class="space-y-3 rounded-xl border border-blue-200 p-4 dark:border-blue-900" data-testid="iq-bootstrap-settings">
  <legend class="px-1 text-sm font-semibold">导入后自动糖果验证</legend>
  <label class="flex items-center gap-2 text-sm"><input type="checkbox" :checked="modelValue.enabled" :disabled="platform!=='openai'" @change="set('enabled',($event.target as HTMLInputElement).checked)" />新 OpenAI 2FA 账号先验证再开放调度</label>
  <p class="text-xs leading-5 text-gray-500">仅作用于后续新建的适用账号；导入后固定 1 并发并关闭客户调度，后台测试继续。重登和重复导入不会重新初始化。</p>
  <div v-if="modelValue.enabled" class="space-y-3">
   <div class="grid gap-3 sm:grid-cols-2">
    <label class="text-sm">测试模型<input class="input mt-1 w-full" :value="modelValue.model" required maxlength="200" @input="set('model',($event.target as HTMLInputElement).value)" /></label>
    <label class="text-sm">思考等级<select class="input mt-1 w-full" :value="modelValue.reasoning" @change="set('reasoning',($event.target as HTMLSelectElement).value)"><option value="low">低（low）</option><option value="minimal">最低（minimal，模型需支持）</option><option value="medium">中</option><option value="high">高</option><option value="xhigh">最高</option></select></label>
    <label class="text-sm">完成后等待（秒）<input class="input mt-1 w-full" type="number" min="1" max="86400" :value="modelValue.interval_seconds" @input="set('interval_seconds',Number(($event.target as HTMLInputElement).value))" /></label>
    <label class="text-sm">连续答对 21 次数<input class="input mt-1 w-full" type="number" min="1" max="1000" :value="modelValue.correct_required" @input="set('correct_required',Number(($event.target as HTMLInputElement).value))" /></label>
    <label class="text-sm">通过后并发<input class="input mt-1 w-full" type="number" min="2" max="10000" :value="modelValue.passed_concurrency" @input="set('passed_concurrency',Number(($event.target as HTMLInputElement).value))" /></label>
   </div>
   <p class="rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950/40 dark:text-blue-200">连续 {{modelValue.correct_required}} 次完整答对 21 后，自动升至 {{modelValue.passed_concurrency}} 并发并打开调度。普通并发升档从通过后开始计客户成功请求。</p>
   <label class="block text-sm">隔离目标分组<select data-testid="quarantine-group" class="input mt-1 w-full" :value="modelValue.quarantine_group_id" @change="selectGroup(Number(($event.target as HTMLSelectElement).value))"><option :value="0">暂不选择（隔离保持关闭）</option><option v-for="g in groups" :key="g.id" :value="g.id" :disabled="g.platform!=='openai'||g.status!=='active'">{{g.name}} · #{{g.id}} · {{g.platform}} · {{g.status==='active'?'启用':'停用'}}</option></select></label>
   <label class="flex items-center gap-2 text-sm"><input data-testid="quarantine-enabled" type="checkbox" :checked="modelValue.quarantine_enabled" :disabled="!validTarget" @change="set('quarantine_enabled',($event.target as HTMLInputElement).checked)" />连续 6 次完整回答 29，移入所选隔离分组</label>
   <p v-if="!validTarget" class="text-xs text-gray-500">读取已有全部分组，由你选择。请选择启用中的 OpenAI 分组后再开启隔离；不影响首次糖果验证。</p>
   <p class="text-xs leading-5 text-gray-500">隔离会移出全部原分组，保留代理和当前并发、停止自动升档；测试继续，后续答对不会自动恢复。此处配置保存后用于新账号，已有账号沿用导入时规则。</p>
  </div>
 </fieldset>
</template>
