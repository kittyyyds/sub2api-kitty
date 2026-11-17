<template>
  <nav v-if="tabs.length" class="customer-page-tabs" data-customer-page-tabs :aria-label="t('nav.relatedPages')">
    <RouterLink v-for="tab in tabs" :key="tab.path" :to="tab.path" :aria-current="isCurrent(tab.path) ? 'page' : undefined">
      {{ t(tab.labelKey) }}<Icon v-if="tab.standalone" name="externalLink" size="xs" aria-hidden="true" />
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useCustomerPageTabs } from '@/features/customer-console/navGroups'

const { t } = useI18n()
const route = useRoute()
const tabs = useCustomerPageTabs()
const isCurrent = (path: string) => route.path === path || route.path.startsWith(path + '/')
</script>

<style scoped>
.customer-page-tabs { display: flex; width: fit-content; max-width: 100%; margin-bottom: 18px; padding: 4px; gap: 4px; overflow-x: auto; border: 1px solid var(--kc-line); border-radius: 11px; background: var(--kc-panel-muted); }
.customer-page-tabs a { display: inline-flex; flex-shrink: 0; align-items: center; gap: 6px; padding: 8px 14px; border-radius: 8px; font-size: 13px; color: var(--kc-muted); white-space: nowrap; }
.customer-page-tabs a:hover { color: var(--kc-ink); }
.customer-page-tabs a[aria-current=page] { background: var(--kc-panel); color: var(--kc-ink); box-shadow: var(--kc-shadow); font-weight: 600; }
</style>
