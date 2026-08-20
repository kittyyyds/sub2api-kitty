<script setup lang="ts">
import { computed } from 'vue'

import HeyRouteContactDialog, { type HeyRouteContactPlatform } from './HeyRouteContactDialog.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  open: boolean
  copy: HeyRouteCopy['support']
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
    title: props.copy.dialogQqTitle,
    description: props.copy.dialogQqDescription,
    valueLabel: props.copy.dialogQqNumber,
    value: '1722345668',
    copyLabel: props.copy.dialogCopyAccount,
    qrSrc: '/images/heyroute/support/qq-qr.png',
    qrAlt: props.copy.dialogQqQrAlt,
    qrBackground: '#050a10',
  },
  {
    id: 'wechat',
    label: props.copy.dialogWechatTab,
    icon: 'chatBubble',
    title: props.copy.dialogWechatTitle,
    description: props.copy.dialogWechatDescription,
    valueLabel: props.copy.dialogWechatId,
    value: 'ALe990411',
    copyLabel: props.copy.dialogCopyAccount,
    qrSrc: '/images/heyroute/support/wechat-qr.png',
    qrAlt: props.copy.dialogWechatQrAlt,
    qrBackground: '#ffffff',
  },
])
</script>

<template>
  <HeyRouteContactDialog
    dialog-id="support"
    :open="open"
    :title="copy.dialogTitle"
    :description="copy.dialogDescription"
    :close-label="copy.dialogClose"
    :platform-label="copy.dialogPlatform"
    :platforms="platforms"
    :copied-label="copy.dialogCopied"
    :copy-failed-label="copy.dialogCopyFailed"
    :scan-label="copy.dialogScan"
    :notice="copy.dialogNotice"
    @close="emit('close')"
  />
</template>
