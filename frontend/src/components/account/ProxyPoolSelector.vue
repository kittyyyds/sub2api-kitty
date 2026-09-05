<template>
  <select
    multiple
    :value="modelValue.map(String)"
    :disabled="disabled"
    class="input min-h-24"
    @change="handleChange"
  >
    <option v-for="proxy in proxies" :key="proxy.id" :value="String(proxy.id)">
      {{ proxy.name }} ({{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }})
    </option>
  </select>
</template>

<script setup lang="ts">
import type { Proxy } from '@/types'

defineProps<{
  modelValue: number[]
  proxies: Proxy[]
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const handleChange = (event: Event) => {
  const select = event.target as HTMLSelectElement
  emit('update:modelValue', Array.from(select.selectedOptions, (option) => Number(option.value)).filter((id) => id > 0))
}
</script>
