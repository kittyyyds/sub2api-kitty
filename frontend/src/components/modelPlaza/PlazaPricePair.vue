<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { formatCnyAmount, formatCnyScaled, formatScaled } from '@/utils/pricing'

const props = withDefaults(defineProps<{
  referencePrice: number | null
  chargedPrice: number | null
  scale?: number
  priceCurrency?: 'usd' | 'cny' | 'balance'
}>(), {
  scale: 1,
  priceCurrency: 'usd',
})

const { t } = useI18n()

const discounted = computed(() => (
  props.referencePrice != null
  && props.chargedPrice != null
  && props.chargedPrice < props.referencePrice - Number.EPSILON
))

function formatPrice(value: number | null): string {
  if (props.priceCurrency === 'balance') return formatScaled(value, props.scale, 2)
  return props.priceCurrency === 'cny'
    ? formatCnyAmount(value, props.scale, 2)
    : formatCnyScaled(value, props.scale, 2)
}
</script>

<template>
  <div v-if="chargedPrice != null || referencePrice != null" class="price-pair">
    <strong v-if="chargedPrice != null" class="customer-price">
      {{ formatPrice(chargedPrice) }}
    </strong>
    <strong v-else class="customer-price">-</strong>
    <span v-if="referencePrice != null" class="reference-price" :class="{ discounted }">
      {{ t('modelPlaza.table.officialShort') }} {{ formatPrice(referencePrice) }}
    </span>
  </div>
  <span v-else class="empty-price">-</span>
</template>

<style scoped>
.price-pair {
  display: grid;
  min-height: 44px;
  align-content: center;
  gap: 3px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-variant-numeric: tabular-nums;
}

.reference-price {
  color: var(--plaza-reference);
  font-family: ui-sans-serif, system-ui, sans-serif;
  font-size: 10px;
  line-height: 1.2;
  white-space: nowrap;
}

.reference-price.discounted {
  text-decoration: line-through;
  text-decoration-color: var(--plaza-reference-strike);
}

.customer-price {
  color: var(--plaza-price);
  font-size: 15px;
  font-weight: 800;
  line-height: 1.25;
  white-space: nowrap;
}

.empty-price {
  color: var(--plaza-muted);
}
</style>
