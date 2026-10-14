<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getTwoFABrowserFrame, sendTwoFABrowserInput, resumeTwoFABrowser } from '@/api/admin/accountTokenGuard'
const props = defineProps<{jobId:string}>()
const emit = defineEmits<{close:[]}>()
const frame = ref(''), text = ref(''), error = ref(''), sending = ref(false)
let disposed = false, loading = false, timer: ReturnType<typeof setTimeout> | undefined
async function refresh() {
 if (disposed || loading) return
 loading = true
 try {
  const blob = await getTwoFABrowserFrame(props.jobId)
  if (disposed) return
  const next = URL.createObjectURL(blob), old = frame.value
  frame.value = next
  error.value = ''
  if (old) URL.revokeObjectURL(old)
 } catch { if (!disposed) error.value = '窗口尚未就绪或已结束；可以关闭此窗口查看导入结果。' }
 finally { loading = false; if (!disposed) timer = setTimeout(refresh, 1200) }
}
async function input(value: Parameters<typeof sendTwoFABrowserInput>[1]) {
 if (sending.value || disposed) return
 sending.value = true; error.value = ''
 try { await sendTwoFABrowserInput(props.jobId, value) }
 catch { error.value = '操作未完成，请稍后重试。' }
 finally { sending.value = false }
}
function click(event: MouseEvent) {
 const image = event.currentTarget as HTMLImageElement, box = image.getBoundingClientRect()
 void input({type:'click',x:Math.min(1023,Math.max(0,(event.clientX-box.left)*1024/box.width)),y:Math.min(767,Math.max(0,(event.clientY-box.top)*768/box.height))})
}
async function sendText() { const value = text.value; text.value = ''; await input({type:'text',text:value}) }
async function resume() {
 error.value = ''
 try { await resumeTwoFABrowser(props.jobId) }
 catch { error.value = '会话尚未就绪或已结束，请查看导入结果。' }
}
onMounted(refresh)
onBeforeUnmount(() => {disposed=true;clearTimeout(timer);text.value='';if(frame.value)URL.revokeObjectURL(frame.value)})
</script>
<template>
 <BaseDialog :show="true" title="服务器登录 / 安全验证" width="extra-wide" :close-on-click-outside="false" @close="emit('close')">
  <div class="space-y-3">
   <p class="input-hint">登录窗口运行在服务器，沿用本次任务的代理和时区。请直接点击画面完成官方要求的验证；关闭此窗口不会取消登录。</p>
   <p v-if="error" role="alert" class="text-sm text-amber-600">{{ error }}</p>
   <img v-if="frame" :src="frame" alt="服务器上的官方登录页面" class="w-full rounded-lg border" draggable="false" @click="click" @wheel.prevent="input({type:'wheel',delta:Math.sign($event.deltaY)*400})" />
   <p v-else class="input-hint">正在获取登录窗口…</p>
   <div class="flex flex-wrap items-center gap-2">
    <input v-model="text" type="password" autocomplete="off" class="input flex-1" aria-label="发送到登录窗口的文本" placeholder="需要手动输入时，先点击画面中的输入框" @keydown.enter.prevent="sendText" />
    <button type="button" class="btn btn-secondary" :disabled="sending || !text" @click="sendText">发送文本</button>
    <button v-for="key in ['Tab','Enter','Backspace','Control+a']" :key="key" type="button" class="btn btn-secondary" :disabled="sending" @click="input({type:'key',key})">{{ key === 'Control+a' ? '全选' : key }}</button>
   </div>
   <div class="flex justify-end gap-2">
    <button type="button" class="btn btn-secondary" @click="emit('close')">关闭窗口</button>
    <button type="button" class="btn btn-primary" @click="resume">我已完成验证，继续登录</button>
   </div>
  </div>
 </BaseDialog>
</template>
