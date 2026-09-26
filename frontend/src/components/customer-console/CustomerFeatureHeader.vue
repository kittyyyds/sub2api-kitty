<script setup lang="ts">
import { computed } from 'vue'

import Icon from '@/components/icons/Icon.vue'

type IconName = NonNullable<InstanceType<typeof Icon>['$props']['name']>

interface FeatureMetric {
  label: string
  value: string
}

const props = defineProps<{
  eyebrow: string
  title: string
  description: string
  metrics: Array<FeatureMetric>
  icon?: IconName
}>()

const resolvedIcon = computed<IconName>(() => props.icon ?? 'sparkles')
</script>

<template>
  <header class="feature-header">
    <div class="feature-header__lead">
      <div class="feature-header__mark" aria-hidden="true">
        <Icon :name="resolvedIcon" size="lg" :stroke-width="1.7" />
      </div>

      <div class="feature-header__copy">
        <div class="feature-header__eyebrow">
          <span class="feature-header__eyebrow-line"></span>
          <span>{{ eyebrow }}</span>
        </div>
        <h1 class="feature-header__title">{{ title }}</h1>
        <p class="feature-header__description">{{ description }}</p>
      </div>
    </div>

    <div v-if="metrics.length" class="feature-header__metrics">
      <div v-for="metric in metrics" :key="metric.label">
        <strong>{{ metric.value }}</strong>
        <span>{{ metric.label }}</span>
      </div>
    </div>
  </header>
</template>

<style scoped>
.feature-header {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px 32px;
  min-width: 0;
  padding: 24px;
  overflow: hidden;
  color: var(--customer-ink, #142126);
  background:
    linear-gradient(112deg, var(--customer-cyan-soft, rgba(10, 127, 144, 0.1)), transparent 46%),
    linear-gradient(292deg, var(--customer-copper-soft, rgba(156, 100, 31, 0.11)), transparent 38%),
    var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 8px;
  box-shadow: var(--customer-shadow, 0 14px 40px rgba(25, 47, 55, 0.1));
}

.feature-header::before {
  position: absolute;
  top: 0;
  right: 0;
  width: 128px;
  height: 3px;
  content: '';
  background: linear-gradient(
    90deg,
    var(--customer-cyan, #0a7f90),
    var(--customer-copper, #9c641f)
  );
}

.feature-header__lead {
  display: flex;
  gap: 16px;
  min-width: 0;
}

.feature-header__mark {
  display: grid;
  flex: 0 0 48px;
  width: 48px;
  height: 48px;
  color: var(--customer-cyan, #0a7f90);
  place-items: center;
  background: var(--customer-cyan-soft, rgba(10, 127, 144, 0.1));
  border: 1px solid var(--customer-line-strong, rgba(10, 127, 144, 0.36));
  border-radius: 6px;
}

.feature-header__copy {
  min-width: 0;
}

.feature-header__eyebrow {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 6px;
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1.4;
  text-transform: uppercase;
}

.feature-header__eyebrow-line {
  width: 18px;
  height: 1px;
  background: currentColor;
}

.feature-header__title {
  margin: 0;
  color: var(--customer-ink, #142126);
  font-size: 30px;
  font-weight: 680;
  letter-spacing: 0;
  line-height: 1.2;
}

.feature-header__description {
  max-width: 760px;
  margin: 8px 0 0;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 14px;
  line-height: 1.75;
}

.feature-header__metrics {
  display: grid;
  grid-column: 1 / -1;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  padding-top: 16px;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.feature-header__metrics :deep(> *) {
  min-width: 0;
  padding: 0 16px;
  border-left: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.feature-header__metrics :deep(> *:first-child) {
  padding-left: 0;
  border-left: 0;
}

.feature-header__metrics :deep(strong) {
  display: block;
  overflow: hidden;
  color: var(--customer-ink, #142126);
  font-family: var(--customer-mono, monospace);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.feature-header__metrics :deep(span) {
  display: block;
  margin-top: 2px;
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-size: 11px;
  line-height: 1.4;
}

@media (max-width: 720px) {
  .feature-header {
    grid-template-columns: 1fr;
    gap: 16px;
    padding: 18px;
  }

  .feature-header__mark {
    flex-basis: 42px;
    width: 42px;
    height: 42px;
  }

  .feature-header__title {
    font-size: 24px;
  }

  .feature-header__metrics {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .feature-header__metrics :deep(> *) {
    padding: 0 0 10px;
    border-bottom: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
    border-left: 0;
  }

  .feature-header__metrics :deep(> *:last-child) {
    padding-bottom: 0;
    border-bottom: 0;
  }
}
</style>
