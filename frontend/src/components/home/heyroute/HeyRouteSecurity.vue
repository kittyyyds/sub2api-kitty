<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['security']
  dashboardPath: string
}

defineProps<Props>()

const cardKeys = ['soc2', 'privacy', 'support'] as const
const cardTones = ['light', 'soft', 'dark'] as const
</script>

<template>
  <section id="hr-security" class="hr-security hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-security-shell">
      <header class="hr-security-head">
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
      </header>

      <div class="hr-security-grid">
        <article
          v-for="(item, index) in copy.items"
          :key="item.title"
          class="hr-security-card"
          :data-card="cardKeys[index]"
          :data-tone="cardTones[index]"
        >
          <span class="hr-security-mark" aria-hidden="true">
            <span />
            <span />
            <span />
          </span>

          <div class="hr-security-card-copy">
            <div class="hr-security-title-row">
              <h3>{{ item.title }}</h3>
              <router-link v-if="index === 2" :to="dashboardPath" class="hr-security-action">
                {{ copy.action }}
                <Icon name="arrowRight" size="sm" />
              </router-link>
            </div>
            <p>{{ item.description }}</p>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-security {
  background:
    radial-gradient(circle at 18% 18%, rgba(198, 179, 244, 0.32), transparent 34%),
    radial-gradient(circle at 82% 24%, rgba(251, 183, 164, 0.28), transparent 32%),
    radial-gradient(circle at 65% 92%, rgba(165, 210, 190, 0.26), transparent 36%),
    var(--hr-bg);
}

.hr-security-shell {
  display: grid;
  gap: 72px;
}

.hr-security-head {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(280px, 0.65fr);
  gap: 72px;
  align-items: end;
}

.hr-security-head h2 {
  max-width: 11em;
  margin: 0;
  font-size: 4.6rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.94;
  text-wrap: balance;
}

.hr-security-head > p {
  max-width: 560px;
  justify-self: end;
  margin: 0;
  color: var(--hr-muted);
  font-size: 17px;
  line-height: 1.65;
}

.hr-security-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

.hr-security-card {
  position: relative;
  isolation: isolate;
  display: flex;
  min-height: 410px;
  flex-direction: column;
  justify-content: space-between;
  overflow: hidden;
  border: 1px solid var(--hr-glass-border);
  border-radius: 28px;
  padding: 34px;
  color: #171719;
  background: rgba(240, 234, 250, 0.42);
  box-shadow: 0 28px 64px var(--hr-shadow-soft), inset 0 0 0 1px rgba(255, 255, 255, 0.28);
}

.hr-security-card[data-tone='soft'] {
  background: rgba(224, 239, 233, 0.4);
}

.hr-security-card[data-tone='dark'] {
  border-color: rgba(255, 255, 255, 0.22);
  color: #fff;
  background: linear-gradient(145deg, rgba(22, 22, 25, 0.88), rgba(43, 39, 45, 0.74));
  box-shadow: 0 34px 78px rgba(36, 29, 31, 0.2), inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.hr-security-mark {
  position: relative;
  display: block;
  width: 48px;
  height: 48px;
  opacity: 0.48;
  transform: translateZ(20px);
}

.hr-security-mark > span {
  position: absolute;
  inset: 0;
  border: 1px solid currentColor;
  border-radius: 50%;
}

.hr-security-mark > span:nth-child(2) {
  transform: translateX(9px);
}

.hr-security-mark > span:nth-child(3) {
  transform: translateY(9px);
}

.hr-security-card-copy {
  position: relative;
  z-index: 2;
  min-width: 0;
  transform: translateZ(28px);
}

.hr-security-title-row {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
}

.hr-security-card h3 {
  max-width: 11ch;
  margin: 0;
  font-size: 31px;
  font-weight: 650;
  letter-spacing: 0;
  line-height: 1.05;
}

.hr-security-card[data-card='privacy'] h3,
.hr-security-card[data-card='support'] h3 {
  max-width: none;
}

.hr-security-card[data-card='support'] h3 {
  font-size: 28px;
}

.hr-security-card p {
  max-width: 390px;
  margin: 18px 0 0;
  color: rgba(23, 23, 25, 0.64);
  font-size: 14px;
  line-height: 1.7;
}

.hr-security-card[data-tone='dark'] p {
  color: rgba(255, 255, 255, 0.68);
}

.hr-security-action {
  display: inline-flex;
  width: max-content;
  min-height: 40px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  border: 1px solid rgba(255, 255, 255, 0.28);
  border-radius: 12px;
  padding: 0 13px;
  color: #fff;
  background: rgba(255, 255, 255, 0.08);
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
  text-decoration: none;
}

.hr-security-action:hover {
  background: rgba(255, 255, 255, 0.16);
  transform: translateY(-2px);
}

.hr-security-action:focus-visible {
  outline: 2px solid #fff;
  outline-offset: 3px;
}

:global(.heyroute-preview[data-theme='dark'] .hr-security) {
  background:
    radial-gradient(circle at 18% 18%, rgba(114, 89, 222, 0.22), transparent 34%),
    radial-gradient(circle at 82% 24%, rgba(219, 105, 78, 0.2), transparent 32%),
    radial-gradient(circle at 65% 92%, rgba(38, 164, 170, 0.16), transparent 36%),
    var(--hr-bg);
}

:global(.heyroute-preview[data-theme='dark'] .hr-security-card[data-tone='light']) {
  color: #f2f1f4;
  background: linear-gradient(145deg, rgba(76, 56, 135, 0.5), rgba(34, 41, 91, 0.52));
}

:global(.heyroute-preview[data-theme='dark'] .hr-security-card[data-tone='soft']) {
  color: #f2f1f4;
  background: linear-gradient(145deg, rgba(24, 93, 99, 0.48), rgba(27, 57, 79, 0.54));
}

:global(.heyroute-preview[data-theme='dark'] .hr-security-card[data-tone='light'] p),
:global(.heyroute-preview[data-theme='dark'] .hr-security-card[data-tone='soft'] p) {
  color: rgba(232, 235, 247, 0.68);
}

@media (max-width: 1000px) {
  .hr-security-head {
    grid-template-columns: 1fr;
    gap: 28px;
  }

  .hr-security-head > p {
    justify-self: start;
  }

  .hr-security-grid {
    grid-template-columns: 1fr;
  }

  .hr-security-card {
    min-height: 330px;
  }
}

@media (max-width: 720px) {
  .hr-security-shell {
    gap: 46px;
  }

  .hr-security-head h2 {
    max-width: 11ch;
    font-size: 2.5rem;
  }

  .hr-security-head > p {
    font-size: 15px;
  }

  .hr-security-card {
    min-height: 300px;
    border-radius: 24px;
    padding: 28px;
  }

  .hr-security-title-row {
    display: grid;
    justify-items: start;
    gap: 18px;
  }

  .hr-security-card h3,
  .hr-security-card[data-card='support'] h3 {
    font-size: 27px;
  }
}
</style>
