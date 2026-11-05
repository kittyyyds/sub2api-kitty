<template>
  <img v-if="url" :src="url" :alt="title" class="h-full w-full object-contain" :title="t('optimizations.posterLabel')" @error="clear">
  <div v-else class="flex h-full items-center justify-center p-4 text-center text-xs text-gray-500" role="status">
    {{ t(pending ? 'optimizations.posterLoading' : 'optimizations.posterFallback') }}
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getPelicanPoster } from '@/utils/pelicanPoster'
const props = defineProps<{ html: string; title: string }>()
const { t } = useI18n()
const url = ref('')
const pending = ref(false)
let request: AbortController | undefined
function clear() { if (url.value) URL.revokeObjectURL(url.value); url.value = '' }
watch(() => props.html, async html => {
  request?.abort(); clear()
  const controller = new AbortController(); request = controller
  pending.value = true
  try {
    const blob = await getPelicanPoster(html, controller.signal)
    if (!controller.signal.aborted && blob) url.value = URL.createObjectURL(blob)
  } catch { /* A still preview failure never hides the original artwork. */ }
  finally { if (!controller.signal.aborted) pending.value = false }
}, { immediate: true })
onBeforeUnmount(() => { request?.abort(); clear() })
</script>
