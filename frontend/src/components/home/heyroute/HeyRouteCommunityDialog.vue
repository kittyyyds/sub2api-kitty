<script setup lang="ts">
import { computed } from 'vue'
import { communityGroupContact, telegramGroupContact } from '@/config/customerContact'

import HeyRouteContactDialog, { type HeyRouteContactPlatform } from './HeyRouteContactDialog.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  open: boolean
  copy: HeyRouteCopy['footer']
}

const props = defineProps<Props>()

const emit = defineEmits<{
  close: []
}>()

const platforms = computed<HeyRouteContactPlatform[]>(() => [
  {
    id: 'qq',
    label: 'QQ',
    icon: 'chat',
    title: props.copy.communityDialogQqTitle,
    description: props.copy.communityDialogQqDescription,
    valueLabel: communityGroupContact.valueKind === 'number'
      ? props.copy.communityDialogGroupNumber
      : props.copy.communityDialogGroupLink,
    value: communityGroupContact.value,
    copyLabel: communityGroupContact.valueKind === 'number'
      ? props.copy.communityDialogCopy
      : props.copy.communityDialogCopyLink,
    externalLabel: props.copy.communityDialogOpenQq,
    externalUrl: communityGroupContact.href,
    qrSrc: communityGroupContact.qrSrc,
    qrAlt: props.copy.communityDialogQqQrAlt,
    qrBackground: '#060b10',
  },
  {
    id: 'telegram',
    label: 'Telegram',
    icon: 'paperAirplane',
    title: props.copy.communityDialogTelegramTitle,
    description: props.copy.communityDialogTelegramDescription,
    valueLabel: props.copy.communityDialogGroupLink,
    value: 't.me/kittyapi1',
    externalLabel: props.copy.communityDialogOpenTelegram,
    externalUrl: telegramGroupContact.href,
    qrSrc: telegramGroupContact.qrSrc,
    qrAlt: props.copy.communityDialogTelegramQrAlt,
    qrBackground: '#060b10',
  },
])
</script>

<template>
  <HeyRouteContactDialog
    dialog-id="community"
    :open="open"
    :title="copy.communityDialogTitle"
    :description="copy.communityDialogDescription"
    :close-label="copy.communityDialogClose"
    :platform-label="copy.communityDialogPlatform"
    :platforms="platforms"
    :copied-label="copy.communityDialogCopied"
    :copy-failed-label="copy.communityDialogCopyFailed"
    :scan-label="copy.communityDialogScan"
    :notice="copy.communityDialogNotice"
    @close="emit('close')"
  />
</template>
