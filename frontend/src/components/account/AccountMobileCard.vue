<template>
  <article
    class="account-mobile-card"
    :class="{ 'is-selected': selected, 'is-error': account.status === 'error', 'is-inactive': account.status === 'inactive' }"
    :data-account-id="account.id"
    data-testid="account-mobile-card"
  >
    <header class="account-mobile-head">
      <div v-if="has('select')" class="account-mobile-select" data-field="select"><Cell column-key="select" /></div>
      <div class="account-mobile-identity">
        <div v-if="has('name')" class="account-mobile-name" data-field="name"><Cell column-key="name" /></div>
        <div class="account-mobile-meta">
          <span v-if="has('id')" data-field="id"><Cell column-key="id" /></span>
          <div v-if="has('platform_type')" class="min-w-0" data-field="platform_type"><Cell column-key="platform_type" /></div>
        </div>
      </div>
    </header>

    <!-- Reuse the original status cell: IQ history stays directly below its status. -->
    <section v-if="has('status')" class="account-mobile-status account-mobile-cell" data-field="status">
      <Cell column-key="status" />
    </section>

    <dl v-if="compactColumns.length" class="account-mobile-stats">
      <div v-for="column in compactColumns" :key="column.key" class="account-mobile-stat account-mobile-cell" :data-field="column.key">
        <dt>{{ column.label }}</dt>
        <dd><Cell :column-key="column.key" /></dd>
      </div>
    </dl>

    <!-- Quotas, model price tables and daily/lifetime usage need the full card width. -->
    <section v-for="column in wideColumns" :key="column.key" class="account-mobile-section account-mobile-cell" :data-field="column.key">
      <h3>{{ column.label }}</h3>
      <Cell :column-key="column.key" />
    </section>

    <dl v-if="detailColumns.length" class="account-mobile-details">
      <div v-for="column in detailColumns" :key="column.key" class="account-mobile-detail account-mobile-cell" :class="{ 'is-wide': column.key === 'notes' || column.key === 'scheduler_score' }" :data-field="column.key">
        <dt>{{ column.label }}</dt>
        <dd><Cell :column-key="column.key" /></dd>
      </div>
    </dl>

    <footer v-if="has('actions')" class="account-mobile-actions" data-field="actions"><Cell column-key="actions" /></footer>
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, type VNodeChild } from 'vue'
import type { Column } from '@/components/common/types'
import type { AccountListItem } from '@/types'

const props = defineProps<{
  account: Pick<AccountListItem, 'id' | 'status'>
  columns: Column[]
  selected?: boolean
  renderCell: (key: string) => VNodeChild
}>()

const Cell = defineComponent({
  props: { columnKey: { type: String, required: true } },
  setup: cellProps => () => props.renderCell(cellProps.columnKey)
})
const has = (key: string) => props.columns.some(column => column.key === key)
const compactKeys = ['rate_multiplier', 'priority', 'schedulable']
const wideKeys = ['capacity', 'upstream_billing_rate', 'today_stats', 'usage', 'groups', 'recent_requests']
const handledKeys = new Set(['select', 'name', 'id', 'platform_type', 'status', 'actions', ...compactKeys, ...wideKeys])
const compactColumns = computed(() => compactKeys.flatMap(key => props.columns.filter(column => column.key === key)))
const wideColumns = computed(() => wideKeys.flatMap(key => props.columns.filter(column => column.key === key)))
// Unknown future columns remain visible instead of silently disappearing on phones.
const detailColumns = computed(() => props.columns.filter(column => !handledKeys.has(column.key)))
</script>

<style scoped>
.account-mobile-card {
  min-width: 0;
  border: 1px solid var(--kc-line);
  border-top: 3px solid rgb(var(--kc-primary-400));
  border-radius: 16px;
  background: var(--kc-panel);
  color: var(--kc-ink);
  box-shadow: var(--kc-shadow);
  overflow-wrap: anywhere;
}
.account-mobile-card.is-selected { border-color: rgb(var(--kc-primary-500)); }
.account-mobile-card.is-error { border-top-color: #ef4444; }
.account-mobile-card.is-inactive { border-top-color: var(--kc-line-strong); }
.account-mobile-head { display: flex; align-items: flex-start; gap: 10px; padding: 15px 14px 10px; }
.account-mobile-select { flex-shrink: 0; padding-top: 2px; }
.account-mobile-select :deep(input) { width: 16px; height: 16px; cursor: pointer; }
.account-mobile-identity { flex: 1; min-width: 0; }
.account-mobile-name { min-width: 0; font-size: 15px; line-height: 1.45; }
.account-mobile-name :deep(a), .account-mobile-name :deep(span) { max-width: 100%; }
.account-mobile-name :deep(.font-medium) { font-weight: 650; }
.account-mobile-meta { display: flex; align-items: flex-start; flex-wrap: wrap; gap: 6px 10px; margin-top: 7px; }
.account-mobile-status { padding: 0 14px 12px; }
.account-mobile-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(76px, 1fr)); gap: 8px; padding: 0 14px 12px; }
.account-mobile-stat { min-width: 0; border-radius: 10px; padding: 9px; background: var(--kc-panel-muted); }
.account-mobile-stat dt, .account-mobile-detail dt, .account-mobile-section h3 { margin-bottom: 6px; font-size: 11px; font-weight: 500; line-height: 1.5; color: var(--kc-muted); }
.account-mobile-stat dd { min-width: 0; font-size: 14px; }
.account-mobile-section { min-width: 0; margin: 0 14px; padding: 11px 0; border-top: 1px solid var(--kc-line); }
.account-mobile-section[data-field='capacity'] :deep(> div) { flex-direction: row; flex-wrap: wrap; gap: 6px; }
.account-mobile-section[data-field='groups'] :deep(> div) { max-width: 100%; }
.account-mobile-section[data-field='usage'] :deep(> div) { max-width: 100%; }
.account-mobile-details { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin: 0 14px; padding: 12px 0; border-top: 1px solid var(--kc-line); }
.account-mobile-detail { min-width: 0; }
.account-mobile-detail.is-wide { grid-column: 1 / -1; }
.account-mobile-detail dd { min-width: 0; font-size: 12px; }
.account-mobile-cell :deep(.whitespace-nowrap) { white-space: normal; }
.account-mobile-cell :deep(.max-w-xs) { max-width: 100%; }
.account-mobile-actions { padding: 8px 12px; border-top: 1px solid var(--kc-line); background: var(--kc-panel-muted); border-radius: 0 0 15px 15px; }
.account-mobile-actions :deep(> div) { justify-content: flex-end; gap: 12px; }
</style>
