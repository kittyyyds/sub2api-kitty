<template>
  <ol class="mb-5 flex items-center gap-2" :aria-label="t('admin.accounts.vmManaged.wizard.stepsAria')" data-testid="vm-steps">
    <template v-for="(label, index) in labels" :key="label">
      <li
        class="flex min-w-0 items-center gap-2"
        :aria-current="index + 1 === current ? 'step' : undefined"
        :data-state="index + 1 < current ? 'done' : index + 1 === current ? 'current' : 'todo'"
      >
        <span
          :class="[
            'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold',
            index + 1 < current
              ? 'bg-primary-500 text-white'
              : index + 1 === current
                ? 'bg-primary-500 text-white ring-4 ring-primary-100 dark:ring-primary-900/40'
                : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
          ]"
        >
          <Icon v-if="index + 1 < current" name="check" size="xs" />
          <template v-else>{{ index + 1 }}</template>
        </span>
        <span
          :class="[
            'hidden truncate text-sm sm:inline',
            index + 1 === current ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-500 dark:text-gray-400'
          ]"
        >{{ t(`admin.accounts.vmManaged.wizard.${label}`) }}</span>
      </li>
      <li v-if="index < labels.length - 1" aria-hidden="true" :class="['h-px min-w-4 flex-1', index + 1 < current ? 'bg-primary-400' : 'bg-gray-200 dark:bg-dark-600']" />
    </template>
  </ol>
  <p v-if="current <= labels.length" class="-mt-3 mb-4 text-xs text-gray-500 sm:hidden dark:text-gray-400">{{ current }} / {{ labels.length }} · {{ t(`admin.accounts.vmManaged.wizard.${labels[current - 1]}`) }}</p>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ current: 1 | 2 | 3 | 4 | 5 }>()
const { t } = useI18n()
const labels = ['settings', 'resources', 'login', 'review'] as const
</script>
