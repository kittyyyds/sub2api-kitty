<template>
  <div class="space-y-4">
    <p class="text-sm text-gray-500">{{ t('qqbot.workspace.groupPolicyHelp') }}</p>
    <article v-for="group in groups" :key="group" class="space-y-3 rounded-lg border p-4 dark:border-dark-700">
      <p v-if="policy(group).label || names?.[group]" class="break-all font-medium">{{ policy(group).label || names?.[group] }}</p>
      <code class="block break-all text-xs">{{ group }}</code>
      <label class="block text-sm">{{ t('qqbot.workspace.groupLabel') }}<input class="input" :value="policy(group).label" :placeholder="names?.[group]" maxlength="60" @input="update(group, { label: ($event.target as HTMLInputElement).value })" /></label>
      <div class="flex flex-wrap gap-4">
        <label v-for="key in switches" :key="key" class="flex items-center gap-2 text-sm"><input type="checkbox" :checked="!policy(group)[key]" @change="update(group, { [key]: !($event.target as HTMLInputElement).checked })" />{{ t(`qqbot.workspace.${key}`) }}</label>
      </div>
      <div class="grid gap-3 sm:grid-cols-2">
        <label v-for="key in quotas" :key="key" class="text-sm">{{ t(`qqbot.workspace.${key}`) }}<input class="input" type="number" min="0" max="100000" :value="policy(group)[key]" :placeholder="t('qqbot.workspace.inherit')" @input="update(group, { [key]: ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value) })" /></label>
      </div>
      <fieldset class="space-y-3 rounded-lg border p-4 dark:border-dark-700" data-testid="qq-join-review">
        <legend class="px-1 text-sm font-medium">{{ t('qqbot.joinReview.title') }}</legend>
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" :checked="joinReview(group).enabled" data-testid="qq-join-review-enabled" @change="updateReview(group, { enabled: ($event.target as HTMLInputElement).checked })" />{{ t('qqbot.joinReview.enabled') }}</label>
        <p class="text-xs text-gray-500">{{ t('qqbot.joinReview.help') }}</p>
        <div v-if="joinReview(group).enabled" class="space-y-3">
          <label class="block text-sm">{{ t('qqbot.joinReview.minSpent') }}<input class="input" type="number" min="0.01" max="1000000000" step="0.01" :value="joinReview(group).min_spent" data-testid="qq-join-review-min-spent" required @input="updateReview(group, { min_spent: Number(($event.target as HTMLInputElement).value) })" /></label>
          <label class="block text-sm">{{ t('qqbot.joinReview.question') }}<input class="input" :value="joinReview(group).question" maxlength="200" data-testid="qq-join-review-question" required @input="updateReview(group, { question: ($event.target as HTMLInputElement).value })" /></label>
          <p class="text-xs text-gray-500">{{ t('qqbot.joinReview.questionHelp') }}</p>
          <label class="block text-sm">{{ t('qqbot.joinReview.rejectReason') }}<input class="input" :value="joinReview(group).reject_reason" maxlength="200" data-testid="qq-join-review-reject-reason" required @input="updateReview(group, { reject_reason: ($event.target as HTMLInputElement).value })" /></label>
          <p class="text-xs text-gray-500">{{ t('qqbot.joinReview.identityHelp') }}</p>
        </div>
      </fieldset>
    </article>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { defaultJoinReviewConfig, type QQGroupPolicy, type QQJoinReviewConfig } from '@/api/qqbot'
const model = defineModel<QQGroupPolicy[]>({ required: true })
defineProps<{ groups: string[]; names?: Record<string, string> }>()
const { t } = useI18n()
const switches = ['disable_chat', 'disable_images', 'disable_activities', 'disable_notifications'] as const
const quotas = ['chat_daily_limit', 'image_daily_limit'] as const
function policy(group: string): QQGroupPolicy { return model.value.find(p => p.group_id === group) || { group_id: group, label: '', disable_chat: false, disable_images: false, disable_activities: false, disable_notifications: false, chat_daily_limit: null, image_daily_limit: null } }
function update(group: string, patch: Partial<QQGroupPolicy>) { const next = { ...policy(group), ...patch }; model.value = [...model.value.filter(p => p.group_id !== group), next] }
function joinReview(group: string): QQJoinReviewConfig { return policy(group).join_review ?? defaultJoinReviewConfig() }
function updateReview(group: string, patch: Partial<QQJoinReviewConfig>) { update(group, { join_review: { ...joinReview(group), ...patch } }) }
</script>
