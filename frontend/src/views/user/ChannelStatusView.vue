<template>
  <ChannelStatusV3View v-if="mode === 'v3'" />
  <ChannelStatusV1View v-else-if="mode === 'v1'" />
  <ChannelStatusV2View v-else />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { isChannelMonitorV1Mode, isChannelMonitorV2Mode, isChannelMonitorV3Mode } from '@/utils/featureFlags'
import ChannelStatusV1View from './ChannelStatusV1View.vue'
import ChannelStatusV2View from './ChannelStatusV2View.vue'
import ChannelStatusV3View from './ChannelStatusV3View.vue'

const mode = computed(() => {
  if (isChannelMonitorV3Mode()) return 'v3'
  if (isChannelMonitorV1Mode()) return 'v1'
  if (isChannelMonitorV2Mode() && new URLSearchParams(window.location.search).get('monitor_view') === 'v2') return 'v2'
  return 'v3'
})
</script>
