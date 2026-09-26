<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { primaryHeyRouteTools } from './heyRouteTools'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['cta']
  dashboardPath: string
}

defineProps<Props>()
</script>

<template>
  <section id="hr-cta" class="hr-final-section hr-content-stage" data-hr-section>
    <div class="hr-section-shell">
      <div class="hr-final-cta" data-hr-tilt="4">
        <div class="hr-cta-dots" aria-hidden="true" />

        <div class="hr-final-head">
          <h2>
            <span>{{ copy.line1 }}</span>
            <span>{{ copy.line2 }}</span>
          </h2>
          <div class="hr-final-action">
            <p>{{ copy.description }}</p>
            <router-link :to="dashboardPath">
              {{ copy.action }}
              <Icon name="arrowRight" size="sm" />
            </router-link>
          </div>
        </div>

        <div class="hr-final-lower">
          <div class="hr-final-tools">
            <div class="hr-final-tools-row">
              <strong class="hr-tools-label">{{ copy.toolsLabel }}</strong>
              <span
                v-for="tool in primaryHeyRouteTools"
                :key="tool.slug"
                class="hr-final-tool-logo"
                :data-tool="tool.slug"
                role="img"
                :aria-label="tool.name"
                :title="tool.name"
              >
                <img :src="tool.src" alt="" loading="lazy" />
              </span>
              <small>{{ copy.moreTools }}</small>
            </div>
          </div>

          <div class="hr-final-signals">
            <div v-for="signal in copy.signals" :key="signal.label" class="hr-final-signal">
              <strong>{{ signal.value }}</strong>
              <span>{{ signal.label }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-final-section {
  position: relative;
  padding: 118px 0 96px;
  background:
    radial-gradient(circle at 50% 46%, rgba(255, 255, 255, 0.52), transparent 58%),
    var(--hr-bg);
}

.hr-final-cta {
  position: relative;
  isolation: isolate;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.68);
  border-radius: 40px;
  padding: 72px;
  color: #171719;
  background:
    radial-gradient(circle at 8% 0%, rgba(255, 235, 170, 0.96), transparent 31%),
    radial-gradient(circle at 98% 10%, rgba(185, 171, 248, 0.84), transparent 34%),
    radial-gradient(circle at 72% 106%, rgba(105, 205, 185, 0.76), transparent 38%),
    radial-gradient(circle at 13% 92%, rgba(255, 143, 127, 0.72), transparent 41%),
    linear-gradient(135deg, #ffc98e, #f7b9cb 48%, #a2d0f1);
  box-shadow: 0 34px 84px var(--hr-shadow), inset 0 0 0 1px rgba(255, 255, 255, 0.24);
}

.hr-cta-dots {
  position: absolute;
  inset: 0;
  z-index: 1;
  opacity: 0.1;
  background-image: radial-gradient(circle, #747478 1px, transparent 1px);
  background-size: 14px 14px;
  mask-image: radial-gradient(circle at 70% 60%, #000, transparent 72%);
  pointer-events: none;
}

.hr-final-head,
.hr-final-lower {
  position: relative;
  z-index: 2;
}

.hr-final-head {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(310px, 0.72fr);
  gap: clamp(48px, 6vw, 96px);
  align-items: end;
}

.hr-final-cta h2 {
  max-width: none;
  margin: 0;
  font-size: 6.25rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.94;
}

.hr-final-cta h2 span {
  display: block;
  white-space: nowrap;
}

.hr-final-action {
  align-self: end;
  max-width: 500px;
  padding-bottom: 6px;
}

.hr-final-action p {
  max-width: 500px;
  margin: 0 0 28px;
  color: rgba(23, 23, 25, 0.68);
  font-size: 17px;
  line-height: 1.65;
}

.hr-final-action a {
  display: inline-flex;
  min-height: 48px;
  align-items: center;
  gap: 14px;
  border-radius: 14px;
  padding: 0 18px;
  color: #fff;
  background: #171719;
  box-shadow: 0 14px 30px rgba(23, 23, 25, 0.18);
  font-size: 13px;
  font-weight: 650;
  text-decoration: none;
}

.hr-final-action a:hover {
  background: #29292c;
  transform: translateY(-2px);
}

.hr-final-action a:focus-visible {
  outline: 2px solid #285f58;
  outline-offset: 3px;
}

.hr-final-lower {
  display: grid;
  grid-template-columns: minmax(420px, 1.08fr) minmax(0, 1fr);
  gap: clamp(48px, 6vw, 88px);
  align-items: end;
  margin-top: 96px;
  border-top: 1px solid rgba(23, 23, 25, 0.16);
  padding-top: 28px;
}

.hr-final-tools {
  min-width: 0;
}

.hr-final-tools-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 7px;
}

.hr-tools-label {
  margin-right: 3px;
  color: rgba(23, 23, 25, 0.7);
  font-size: 12px;
  font-weight: 650;
  line-height: 1.35;
}

.hr-final-tool-logo {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border: 1px solid rgba(255, 255, 255, 0.62);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.25);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14), 0 10px 26px rgba(48, 33, 37, 0.09);
  backdrop-filter: blur(14px) saturate(150%);
  transition: transform 180ms cubic-bezier(.23, 1, .32, 1), background 180ms ease;
}

.hr-final-tool-logo:hover {
  background: rgba(255, 255, 255, 0.48);
  transform: translateY(-3px) rotate(-2deg);
}

.hr-final-tool-logo img {
  display: block;
  width: 23px;
  height: 23px;
  object-fit: contain;
}

.hr-final-tool-logo[data-tool='cursor'] img,
.hr-final-tool-logo[data-tool='hermes'] img {
  border-radius: 5px;
}

.hr-final-tools small {
  margin-left: 6px;
  color: rgba(23, 23, 25, 0.68);
  font-size: 12px;
  font-weight: 600;
}

.hr-final-signals {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.hr-final-signal {
  display: grid;
  min-width: 0;
  gap: 5px;
  padding: 2px 20px 2px 0;
}

.hr-final-signal + .hr-final-signal {
  border-left: 1px solid rgba(23, 23, 25, 0.16);
  padding-left: 20px;
}

.hr-final-signal strong {
  font-size: 15px;
  font-weight: 680;
  letter-spacing: 0;
  line-height: 1.25;
}

.hr-final-signal span {
  color: rgba(23, 23, 25, 0.62);
  font-size: 11px;
  font-weight: 560;
  line-height: 1.35;
}

:global(.heyroute-preview[data-theme='dark'] .hr-final-section) {
  background:
    radial-gradient(circle at 50% 42%, rgba(92, 74, 174, 0.26), transparent 50%),
    radial-gradient(circle at 24% 64%, rgba(191, 84, 83, 0.18), transparent 45%),
    radial-gradient(circle at 80% 58%, rgba(30, 139, 157, 0.18), transparent 42%),
    var(--hr-bg);
}

:global(.heyroute-preview[data-theme='dark'] .hr-final-cta) {
  color: #f5f5fb;
  border-color: rgba(218, 224, 255, 0.32);
  background:
    radial-gradient(circle at 9% 0%, rgba(207, 144, 65, 0.6), transparent 31%),
    radial-gradient(circle at 98% 10%, rgba(121, 87, 223, 0.56), transparent 34%),
    radial-gradient(circle at 72% 106%, rgba(34, 161, 176, 0.46), transparent 38%),
    radial-gradient(circle at 13% 92%, rgba(204, 83, 86, 0.44), transparent 41%),
    linear-gradient(135deg, #182451, #321d4b 48%, #12394a);
}

:global(.heyroute-preview[data-theme='dark'] .hr-final-action p),
:global(.heyroute-preview[data-theme='dark'] .hr-tools-label),
:global(.heyroute-preview[data-theme='dark'] .hr-final-tools small),
:global(.heyroute-preview[data-theme='dark'] .hr-final-signal span) {
  color: rgba(236, 239, 255, 0.72);
}

:global(.heyroute-preview[data-theme='dark'] .hr-final-action a) {
  color: #101321;
  background: #f4f5ff;
}

:global(.heyroute-preview[data-theme='dark'] .hr-final-signal + .hr-final-signal) {
  border-left-color: rgba(222, 228, 255, 0.2);
}

@media (max-width: 1100px) {
  .hr-final-cta h2 {
    font-size: 5rem;
  }

  .hr-final-lower {
    grid-template-columns: 1fr;
    gap: 34px;
  }
}

@media (max-width: 720px) {
  .hr-final-section {
    padding: 78px 0 72px;
  }

  .hr-final-cta {
    border-radius: 28px;
    padding: 44px 24px 36px;
  }

  .hr-final-head {
    grid-template-columns: 1fr;
    gap: 34px;
  }

  .hr-final-cta h2 {
    font-size: 2.35rem;
    line-height: 0.96;
  }

  .hr-final-cta h2 span {
    overflow-wrap: anywhere;
    white-space: normal;
  }

  .hr-final-action {
    padding-bottom: 0;
  }

  .hr-final-action p {
    margin-bottom: 24px;
    font-size: 15px;
  }

  .hr-final-lower {
    margin-top: 54px;
    padding-top: 22px;
  }

  .hr-final-tool-logo {
    width: 34px;
    height: 34px;
    border-radius: 11px;
  }

  .hr-final-tool-logo img {
    width: 21px;
    height: 21px;
  }

  .hr-final-tools small {
    flex-basis: 100%;
    margin: 3px 0 0;
  }

  .hr-final-signal {
    padding-right: 12px;
  }

  .hr-final-signal + .hr-final-signal {
    padding-left: 12px;
  }

  .hr-final-signal strong {
    font-size: 13px;
  }

  .hr-final-signal span {
    font-size: 10px;
  }
}
</style>
