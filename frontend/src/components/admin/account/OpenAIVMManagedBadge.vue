<template>
  <button
    v-if="managed"
    type="button"
    :class="['inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium', tone]"
    :title="t('admin.accounts.vmManaged.title')"
    data-testid="vm-managed-badge"
    @click.stop="emit('open', account)"
  >
    {{ label }}
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { OPENAI_VM_MANAGED_STATUSES, type OpenAIVMManagedStatus } from '@/api/admin/openaiVmManaged'
import type { Account } from '@/types'

// The list row carries the status the state sync last wrote; the panel reads the live one.
const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ (e: 'open', account: Account): void }>()
const { t } = useI18n()

const extra = computed(() => (props.account.extra ?? {}) as Record<string, unknown>)
const managed = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth' && extra.value.openai_vm_managed === true)
const status = computed<OpenAIVMManagedStatus>(() => {
  const value = extra.value.openai_vm_managed_status
  // A new account has no synced status yet: it is waiting for its first login.
  if (typeof value !== 'string' || value === '') return 'awaiting_login'
  return (OPENAI_VM_MANAGED_STATUSES as readonly string[]).includes(value) ? value as OpenAIVMManagedStatus : 'unknown'
})
const label = computed(() => t(`admin.accounts.vmManaged.status.${status.value}`))
const tone = computed(() => {
  switch (status.value) {
    case 'available':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'needs_attention': case 'needs_relogin': case 'error': case 'slot_error':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  }
})
</script>
