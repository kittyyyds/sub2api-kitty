<script setup lang="ts">
import ModelIcon from '@/components/common/ModelIcon.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['scenarios']
}

defineProps<Props>()

const models = ['gpt-5.6', 'claude-sonnet', 'grok-4.5']
</script>

<template>
  <section id="hr-scenarios" class="hr-scenarios hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-scenarios-shell">
      <header>
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
      </header>

      <div class="hr-scenario-grid">
        <article
          v-for="(item, index) in copy.items"
          :key="item.kicker"
          :class="`scenario-${index + 1}`"
        >
          <div class="hr-scenario-topline">
            <div class="hr-scenario-icon"><ModelIcon :model="models[index]" size="54px" /></div>
            <div>
              <span class="hr-scenario-kicker">{{ item.kicker }}</span>
              <strong>{{ item.detail }}</strong>
            </div>
          </div>

          <div class="hr-scenario-copy">
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </div>

          <div class="hr-scenario-telemetry">
            <code>{{ item.flow }}</code>
            <ul :aria-label="item.detail">
              <li v-for="signal in item.signals" :key="signal">{{ signal }}</li>
            </ul>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-scenarios {
  background: var(--hr-bg);
}

.hr-scenarios-shell {
  display: grid;
  gap: 50px;
}

.hr-scenarios header {
  display: grid;
  gap: 18px;
}

.hr-scenarios h2 {
  max-width: 18ch;
  margin: 0;
  font-size: 5.2rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.94;
  text-wrap: balance;
}

.hr-scenarios header p {
  max-width: 860px;
  margin: 0;
  color: var(--hr-muted);
  font-size: 16px;
  line-height: 1.6;
}

.hr-scenario-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
}

.hr-scenario-grid article {
  position: relative;
  display: grid;
  min-height: 430px;
  grid-template-rows: auto 1fr auto;
  gap: 34px;
  overflow: hidden;
  border: 1px solid var(--hr-glass-border);
  border-radius: 8px;
  padding: 28px;
  box-shadow: 0 24px 48px var(--hr-shadow-soft);
}

.scenario-1 {
  grid-column: span 5;
  color: #f2f4f1;
  background: linear-gradient(145deg, #202627, #39434b 56%, #27292f);
}

.scenario-2 {
  grid-column: span 3;
  color: #1b211f;
  background: linear-gradient(155deg, #efb08e, #efcdbd 55%, #bfd1cc);
}

.scenario-3 {
  grid-column: span 4;
  color: #17201e;
  background: linear-gradient(145deg, #b8d4e5, #d2c7e7 48%, #ecb8aa);
}

.hr-scenario-topline {
  display: flex;
  align-items: center;
  gap: 16px;
}

.hr-scenario-icon {
  display: grid;
  width: 76px;
  height: 76px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid rgba(245, 247, 244, 0.4);
  border-radius: 8px;
  background: rgba(245, 247, 244, 0.34);
  backdrop-filter: blur(12px);
}

.hr-scenario-topline > div:last-child {
  display: grid;
  gap: 5px;
}

.hr-scenario-kicker {
  font-size: 13px;
  font-weight: 720;
  opacity: 0.72;
}

.hr-scenario-topline strong {
  font-family: 'Cascadia Code', 'SFMono-Regular', Consolas, monospace;
  font-size: 11px;
  font-weight: 520;
  opacity: 0.58;
}

.hr-scenario-grid h3 {
  max-width: 16ch;
  margin: 0 0 16px;
  font-size: 2.1rem;
  font-weight: 730;
  letter-spacing: 0;
  line-height: 1.04;
}

.hr-scenario-copy p {
  max-width: 52ch;
  margin: 0;
  font-size: 13px;
  font-weight: 520;
  line-height: 1.72;
  opacity: 0.72;
}

.hr-scenario-telemetry {
  display: grid;
  gap: 16px;
  border-top: 1px solid currentColor;
  padding-top: 18px;
  border-top-color: color-mix(in srgb, currentColor 18%, transparent);
}

.hr-scenario-telemetry code {
  font-family: 'Cascadia Code', 'SFMono-Regular', Consolas, monospace;
  font-size: 11px;
  font-weight: 560;
  line-height: 1.45;
  opacity: 0.72;
  overflow-wrap: anywhere;
  white-space: normal;
}

.hr-scenario-telemetry ul {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.hr-scenario-telemetry li {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 11px;
  font-weight: 650;
  opacity: 0.68;
}

.hr-scenario-telemetry li::before {
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 10px currentColor;
  content: '';
}

@media (max-width: 900px) {
  .hr-scenarios h2 {
    max-width: 11ch;
    font-size: 2.5rem;
  }

  .hr-scenario-grid {
    grid-template-columns: 1fr;
  }

  .hr-scenario-grid article {
    min-height: 420px;
    grid-column: auto;
  }

  .hr-scenario-grid h3 {
    font-size: 1.85rem;
  }
}
</style>
