<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import type { GroupPlatform } from '@/types'
import { platformAccentColor, platformLabel } from '@/utils/platformColors'

defineProps<{
  platforms: string[]
  groups: Array<{
    id: number
    name: string
    platform: string
    rate: number
    usesPersonalRate: boolean
  }>
  platform: string
  groupId: number | null
  search: string
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number]
  'update:search': [value: string]
}>()

const { t } = useI18n()

function pricingPlatformLabel(platform: string): string {
  if (platform === 'openai') return t('modelPlaza.platforms.openai')
  if (platform === 'anthropic') return t('modelPlaza.platforms.anthropic')
  if (platform === 'grok') return t('modelPlaza.platforms.grok')
  return t('modelPlaza.platforms.generic', { platform: platformLabel(platform) })
}

function rateLabel(value: number): string {
  return `${Number(value.toFixed(3))}x`
}
</script>

<template>
  <section class="plaza-filters" :aria-label="t('modelPlaza.filters.ariaLabel')">
    <div class="platform-tabs" role="tablist" :aria-label="t('modelPlaza.filters.platformLabel')">
      <button
        v-for="item in platforms"
        :key="item"
        type="button"
        role="tab"
        :aria-selected="platform === item"
        :class="{ active: platform === item }"
        :style="{ '--platform-accent': platformAccentColor(item) }"
        @click="emit('update:platform', item)"
      >
        <PlatformIcon :platform="item as GroupPlatform" size="xs" />
        {{ pricingPlatformLabel(item) }}
      </button>
    </div>

    <div class="group-tools">
      <div class="group-tabs" role="tablist" :aria-label="t('modelPlaza.filters.groupLabel')">
        <button
          v-for="group in groups"
          :key="group.id"
          type="button"
          role="tab"
          :aria-selected="groupId === group.id"
          :class="{ active: groupId === group.id }"
          @click="emit('update:groupId', group.id)"
        >
          <span>{{ group.name }}</span>
          <small>
            {{ group.usesPersonalRate
              ? t('modelPlaza.filters.personalRate', { rate: rateLabel(group.rate) })
              : rateLabel(group.rate) }}
          </small>
        </button>
      </div>

      <label class="model-search">
        <span class="sr-only">{{ t('modelPlaza.filters.modelLabel') }}</span>
        <Icon name="search" size="xs" />
        <input
          :value="search"
          type="search"
          :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          @input="emit('update:search', ($event.target as HTMLInputElement).value)"
        >
        <button
          v-if="search"
          type="button"
          :title="t('modelPlaza.filters.clearSearch')"
          :aria-label="t('modelPlaza.filters.clearSearch')"
          @click="emit('update:search', '')"
        >
          <Icon name="x" size="xs" />
        </button>
      </label>
    </div>
  </section>
</template>

<style scoped>
.plaza-filters {
  padding: 18px 18px 16px;
  border-bottom: 1px solid var(--plaza-line);
  background: var(--plaza-surface);
}

.platform-tabs,
.group-tabs {
  display: flex;
  min-width: 0;
  align-items: center;
  overflow-x: auto;
  scrollbar-color: var(--plaza-line-strong) transparent;
  scrollbar-width: thin;
}

.platform-tabs {
  width: fit-content;
  max-width: 100%;
  margin-inline: auto;
  border: 1px solid var(--plaza-line);
  border-radius: 6px;
  padding: 3px;
  background: var(--plaza-surface-muted);
}

.platform-tabs button,
.group-tabs button {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  color: var(--plaza-muted);
  white-space: nowrap;
}

.platform-tabs button {
  --platform-accent: var(--plaza-accent);
  min-height: 34px;
  gap: 6px;
  border-radius: 4px;
  padding: 0 18px;
  font-size: 11px;
  font-weight: 650;
}

.platform-tabs button:hover {
  color: var(--platform-accent);
}

.platform-tabs button.active {
  border-color: color-mix(in srgb, var(--platform-accent) 52%, var(--plaza-line));
  color: #ffffff;
  background: color-mix(in srgb, var(--platform-accent) 84%, var(--plaza-active-mix));
  font-weight: 750;
}

.group-tools {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(180px, 230px);
  align-items: center;
  gap: 12px;
  max-width: 900px;
  margin: 14px auto 0;
}

.group-tabs {
  justify-content: flex-start;
  gap: 7px;
}

.group-tabs button {
  min-height: 32px;
  gap: 6px;
  border-color: var(--plaza-line);
  border-radius: 5px;
  padding: 0 11px;
  background: var(--plaza-surface);
  font-size: 10px;
}

.group-tabs button:hover {
  border-color: color-mix(in srgb, var(--plaza-accent) 42%, var(--plaza-line));
  color: var(--plaza-accent);
}

.group-tabs button.active {
  border-color: var(--plaza-accent);
  color: var(--plaza-accent);
  background: var(--plaza-accent-soft);
  font-weight: 700;
}

.group-tabs small {
  color: inherit;
  opacity: .78;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 9px;
}

.model-search {
  display: flex;
  height: 34px;
  min-width: 0;
  align-items: center;
  gap: 7px;
  border: 1px solid var(--plaza-line-strong);
  border-radius: 5px;
  padding: 0 8px;
  color: var(--plaza-muted-soft);
  background: var(--plaza-surface-muted);
}

.model-search:focus-within {
  border-color: var(--plaza-accent);
  box-shadow: 0 0 0 3px var(--plaza-accent-ring);
}

.model-search input {
  width: 100%;
  min-width: 0;
  border: 0;
  outline: 0;
  color: var(--plaza-text-strong);
  background: transparent;
  font-size: 10px;
}

.model-search input::placeholder {
  color: var(--plaza-muted-soft);
}

.model-search button {
  display: grid;
  width: 22px;
  height: 22px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 4px;
}

.model-search button:hover {
  color: var(--plaza-accent);
  background: var(--plaza-accent-soft);
}

@media (max-width: 760px) {
  .plaza-filters {
    padding-inline: 12px;
  }

  .platform-tabs {
    width: 100%;
    justify-content: flex-start;
  }

  .platform-tabs button {
    padding-inline: 13px;
  }

  .group-tools {
    grid-template-columns: 1fr;
  }

  .model-search {
    width: 100%;
  }
}
</style>
