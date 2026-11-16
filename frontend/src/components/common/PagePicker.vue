<template>
  <div ref="root" class="relative">
    <button ref="trigger" type="button" class="btn btn-secondary btn-sm tabular-nums" :aria-expanded="open" @click="toggle">
      {{ t(exact ? 'pagination.pageOf' : 'optimizations.pagesAtLeast', { page, total: pages }) }}
      <span aria-hidden="true">▾</span>
    </button>
    <div v-if="open" class="absolute bottom-full left-1/2 z-50 mb-2 w-56 -translate-x-1/2 rounded-xl border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-600 dark:bg-dark-800" @keydown.esc.stop.prevent="close">
      <form class="flex gap-2" @submit.prevent="submit">
        <input v-model="input" type="number" min="1" :max="pages" step="1" class="input min-w-0 flex-1 text-sm" :aria-label="t('pagination.jumpTo')" :placeholder="t('pagination.jumpTo')">
        <button type="submit" class="btn btn-primary btn-sm">{{ t('pagination.jumpAction') }}</button>
      </form>
      <p v-if="!exact" class="my-2 text-xs text-gray-500">{{ t('optimizations.lowerBoundHint') }}</p>
      <div ref="scroller" class="mt-2 max-h-60 overflow-y-auto overscroll-contain" data-testid="page-picker-scroll" @scroll="scrollTop = ($event.target as HTMLElement).scrollTop">
        <div :style="{ height: `${pages * rowHeight}px`, position: 'relative' }">
          <button v-for="n in visible" :key="n" type="button" class="absolute left-0 w-full rounded px-3 text-left text-sm hover:bg-primary-50 dark:hover:bg-dark-700" :style="{ top: `${(n - 1) * rowHeight}px`, height: `${rowHeight}px` }" :class="n === page ? 'font-bold text-primary-600' : 'text-gray-700 dark:text-gray-200'" :aria-current="n === page ? 'page' : undefined" @click="select(n)">
            {{ t('pagination.goToPage', { page: n }) }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
const props = withDefaults(defineProps<{ page: number; pages: number; exact?: boolean }>(), { exact: true })
const emit = defineEmits<{ select: [page: number] }>()
const { t } = useI18n()
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const scroller = ref<HTMLElement>()
const open = ref(false)
const input = ref('')
const scrollTop = ref(0)
const rowHeight = 36
// A fixed-size window, not an array of all page numbers. Handles millions of rows.
const visible = computed(() => {
  const start = Math.max(1, Math.floor(scrollTop.value / rowHeight) - 2)
  const end = Math.min(props.pages, start + 12)
  return Array.from({ length: Math.max(0, end - start + 1) }, (_, i) => start + i)
})
async function toggle() {
  open.value = !open.value
  if (!open.value) return
  input.value = ''
  scrollTop.value = Math.max(0, (props.page - 4) * rowHeight)
  await nextTick()
  if (scroller.value) scroller.value.scrollTop = scrollTop.value
  root.value?.querySelector('input')?.focus()
}
function close() { open.value = false; trigger.value?.focus() }
function select(n: number) {
  if (!Number.isSafeInteger(n) || n < 1 || n > props.pages) return
  if (n !== props.page) emit('select', n)
  close()
}
function submit() { if (String(input.value).trim()) select(Number(input.value)) }
function outside(event: Event) { if (event.target instanceof Node && !root.value?.contains(event.target)) open.value = false }
watch(() => [props.pages, props.page], () => { open.value = false })
onMounted(() => { document.addEventListener('pointerdown', outside); document.addEventListener('focusin', outside) })
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside); document.removeEventListener('focusin', outside) })
</script>
