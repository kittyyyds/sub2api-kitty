<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ enabled: boolean; pool: string }>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:pool': [value: string]
}>()
const { t } = useI18n()
</script>

<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="flex items-center justify-between gap-3">
      <span class="input-label mb-0">{{ t('admin.accounts.tierRouting.enabled') }}</span>
      <input type="checkbox" :checked="enabled" data-testid="account-tier-routing-enabled"
        class="h-4 w-4 rounded border-gray-300 text-primary-600"
        @change="emit('update:enabled', ($event.target as HTMLInputElement).checked)" />
    </label>
    <p class="input-hint">{{ t('admin.accounts.tierRouting.hint') }}</p>
    <label v-if="enabled" class="mt-3 block">
      <span class="input-label">{{ t('admin.accounts.tierRouting.pool') }}</span>
      <select :value="pool" class="input" data-testid="account-tier-routing-pool"
        @change="emit('update:pool', ($event.target as HTMLSelectElement).value)">
        <option value="premium">{{ t('admin.accounts.tierRouting.premium') }}</option>
        <option value="standard">{{ t('admin.accounts.tierRouting.standard') }}</option>
      </select>
    </label>
  </div>
</template>
