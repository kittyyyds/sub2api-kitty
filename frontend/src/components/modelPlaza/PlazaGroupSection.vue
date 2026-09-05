<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { ModelPlazaGroup } from '@/api/modelPlaza'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import type { GroupPlatform } from '@/types'
import { formatPeakRateWindow, hasPeakRate, serverTimezoneLabel } from '@/utils/peak-rate'

import PlazaModelPricingTable from './PlazaModelPricingTable.vue'

const props = defineProps<{
  group: ModelPlazaGroup
}>()

const { t } = useI18n()
const appStore = useAppStore()

const textRate = computed(() => props.group.user_rate_multiplier ?? props.group.rate_multiplier)
const usesPersonalTextRate = computed(() => (
  props.group.user_rate_multiplier != null
  && Math.abs(props.group.user_rate_multiplier - props.group.rate_multiplier) > Number.EPSILON
))
const imageRate = computed(() => (
  props.group.image_rate_independent ? props.group.image_rate_multiplier : textRate.value
))
const videoRate = computed(() => (
  props.group.video_rate_independent ? props.group.video_rate_multiplier : textRate.value
))

const peakWindow = computed(() => {
  if (!hasPeakRate(props.group)) return ''
  return formatPeakRateWindow(
    props.group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
})

const peakNote = computed(() => {
  if (!peakWindow.value) return ''
  return t('modelPlaza.detail.peakNote', {
    window: peakWindow.value,
    multiplier: props.group.peak_rate_multiplier
  })
})

const longContextNote = computed(() => {
  if (props.group.long_context_pricing_enabled !== false) return ''
  const hasOfficialLadder = props.group.models.some(
    (model) => (model.official_pricing?.intervals?.length ?? 0) > 1
  )
  return hasOfficialLadder ? t('modelPlaza.detail.longContextDisabledNote') : ''
})

function rateLabel(value: number): string {
  return `${Number(value.toFixed(3))}x`
}
</script>

<template>
  <section class="pricing-group">
    <header class="pricing-group-header">
      <div class="group-copy">
        <div class="group-title-row">
          <PlatformIcon :platform="group.platform as GroupPlatform" size="sm" />
          <h2>{{ group.name }}</h2>
          <span class="visibility-badge">
            <Icon :name="group.is_exclusive ? 'shield' : 'globe'" size="xs" />
            {{ group.is_exclusive ? t('modelPlaza.badges.exclusive') : t('modelPlaza.badges.public') }}
          </span>
          <span v-if="group.subscription_type === 'subscription'" class="subscription-badge">
            {{ t('modelPlaza.badges.subscription') }}
          </span>
        </div>
        <p>{{ group.description || t('modelPlaza.detail.defaultDescription') }}</p>
      </div>

      <div class="rate-list" :aria-label="t('modelPlaza.detail.effectiveRate')">
        <span>
          {{ t(usesPersonalTextRate ? 'modelPlaza.detail.personalTextRate' : 'modelPlaza.detail.textRate') }}
          <strong>{{ rateLabel(textRate) }}</strong>
        </span>
        <span v-if="imageRate !== textRate">
          {{ t('modelPlaza.detail.imageRate') }}
          <strong>{{ rateLabel(imageRate) }}</strong>
        </span>
        <span v-if="videoRate !== textRate">
          {{ t('modelPlaza.detail.videoRate') }}
          <strong>{{ rateLabel(videoRate) }}</strong>
        </span>
      </div>
    </header>

    <p v-if="usesPersonalTextRate" class="personal-rate-note">
      <Icon name="infoCircle" size="xs" />
      {{ t('modelPlaza.detail.personalRateNote', {
        personalRate: rateLabel(textRate),
        groupRate: rateLabel(group.rate_multiplier)
      }) }}
    </p>

    <p v-if="peakNote" class="peak-note">
      <Icon name="clock" size="xs" />
      {{ peakNote }}
    </p>

    <p v-if="longContextNote" class="personal-rate-note">
      <Icon name="infoCircle" size="xs" />
      {{ longContextNote }}
    </p>

    <PlazaModelPricingTable
      v-if="group.models.length > 0"
      :models="group.models"
      :platform="group.platform"
      :rate-multiplier="group.rate_multiplier"
      :user-rate-multiplier="group.user_rate_multiplier ?? null"
      :image-rate-independent="group.image_rate_independent"
      :image-rate-multiplier="group.image_rate_multiplier"
      :video-rate-independent="group.video_rate_independent"
      :video-rate-multiplier="group.video_rate_multiplier"
      :peak-window="peakWindow"
      :peak-rate-multiplier="group.peak_rate_multiplier"
    />
    <p v-else class="no-models">
      {{ t('modelPlaza.detail.noModels') }}
    </p>
  </section>
</template>

<style scoped>
.pricing-group {
  background: var(--plaza-surface);
}

.pricing-group-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  padding: 18px 22px;
  border-bottom: 1px solid var(--plaza-line-soft);
  background: var(--plaza-surface-muted);
}

.group-copy {
  min-width: 0;
}

.group-title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}

.group-title-row h2 {
  margin: 0;
  color: var(--plaza-text-strong);
  font-size: 13px;
  font-weight: 750;
}

.group-copy > p {
  max-width: 680px;
  margin: 7px 0 0;
  color: var(--plaza-muted);
  font-size: 10px;
  line-height: 1.6;
}

.visibility-badge,
.subscription-badge {
  display: inline-flex;
  min-height: 21px;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--plaza-line);
  border-radius: 4px;
  padding: 0 6px;
  background: var(--plaza-surface);
  color: var(--plaza-muted);
  font-size: 9px;
  font-weight: 600;
}

.subscription-badge {
  color: var(--plaza-subscription);
}

.rate-list {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}

.rate-list span {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 5px;
  border: 1px solid color-mix(in srgb, var(--plaza-accent) 30%, var(--plaza-line));
  border-radius: 5px;
  padding: 0 8px;
  color: var(--plaza-muted);
  background: var(--plaza-surface);
  font-size: 9px;
}

.rate-list strong {
  color: var(--plaza-accent);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
}

.peak-note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  padding: 8px 22px;
  border-bottom: 1px solid var(--plaza-peak-border);
  background: var(--plaza-peak-bg);
  color: var(--plaza-peak-text);
  font-size: 10px;
}

.personal-rate-note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  padding: 8px 22px;
  border-bottom: 1px solid color-mix(in srgb, var(--plaza-accent) 30%, var(--plaza-line));
  background: var(--plaza-accent-soft);
  color: var(--plaza-accent);
  font-size: 10px;
}

.no-models {
  margin: 0;
  padding: 48px 20px;
  color: var(--plaza-muted);
  text-align: center;
  font-size: 11px;
}

@media (max-width: 720px) {
  .pricing-group-header {
    display: block;
    padding-inline: 14px;
  }

  .rate-list {
    justify-content: flex-start;
    margin-top: 12px;
  }

  .peak-note {
    padding-inline: 14px;
  }

  .personal-rate-note {
    padding-inline: 14px;
  }
}
</style>
