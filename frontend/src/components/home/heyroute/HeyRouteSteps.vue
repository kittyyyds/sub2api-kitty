<script setup lang="ts">
import { shallowRef } from 'vue'

import { useHeyRouteStepSequence } from '@/composables/useHeyRouteStepSequence'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['steps']
  apiBaseUrl: string
}

defineProps<Props>()

const sectionRef = shallowRef<HTMLElement | null>(null)
const gridRef = shallowRef<HTMLOListElement | null>(null)

useHeyRouteStepSequence(sectionRef, gridRef)
</script>

<template>
  <section ref="sectionRef" id="hr-steps" class="hr-steps hr-stage hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-steps-shell">
      <header>
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
      </header>

      <ol ref="gridRef" class="hr-step-list">
        <li class="hr-step-item">
          <div class="hr-step-reveal">
            <div class="hr-step-preview preview-key">
              <span>{{ copy.apiKey }}</span>
              <strong>sk-••••••••••••</strong>
              <em>{{ copy.created }}</em>
            </div>
          </div>
          <div class="hr-step-node"><span class="hr-step-index">01</span></div>
          <h3>{{ copy.items[0].title }}</h3>
          <p>{{ copy.items[0].description }}</p>
        </li>
        <li class="hr-step-item">
          <div class="hr-step-reveal">
            <div class="hr-step-preview preview-config">
              <div><span>{{ copy.baseUrl }}</span><strong>{{ apiBaseUrl }}</strong></div>
              <div><span>{{ copy.apiKey }}</span><strong>sk-••••••••</strong></div>
              <div><span>{{ copy.model }}</span><strong>gpt-5.6</strong></div>
            </div>
          </div>
          <div class="hr-step-node"><span class="hr-step-index">02</span></div>
          <h3>{{ copy.items[1].title }}</h3>
          <p>{{ copy.items[1].description }}</p>
        </li>
        <li class="hr-step-item">
          <div class="hr-step-reveal">
            <div class="hr-step-preview preview-request">
              <span>{{ copy.success }}</span>
              <strong>200</strong>
              <code>POST /v1/responses</code>
              <em>{{ copy.response }}</em>
            </div>
          </div>
          <div class="hr-step-node"><span class="hr-step-index">03</span></div>
          <h3>{{ copy.items[2].title }}</h3>
          <p>{{ copy.items[2].description }}</p>
        </li>
      </ol>
    </div>
  </section>
</template>

<style scoped>
.hr-steps {
  background:
    radial-gradient(circle at 8% 14%, rgba(124, 188, 207, 0.15), transparent 30%),
    radial-gradient(circle at 86% 80%, rgba(239, 146, 115, 0.14), transparent 30%),
    var(--hr-bg);
}

.hr-steps-shell {
  display: grid;
  width: min(calc(100% - 80px), 1400px);
  gap: 54px;
}

.hr-steps header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 0.52fr);
  gap: 70px;
  align-items: end;
}

.hr-steps h2 {
  max-width: 13ch;
  margin: 0;
  font-size: 5.2rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.94;
  text-wrap: balance;
}

.hr-steps header p {
  margin: 0 0 8px;
  color: var(--hr-muted);
  font-size: 16px;
  line-height: 1.6;
}

.hr-step-list {
  --hr-step-line-left: 22px;
  --hr-step-line-right: 0px;
  --hr-step-line-y: 306px;
  --hr-step-line-clip: 0%;
  --hr-step-line-fade-start: 90%;
  position: relative;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 32px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.hr-step-list::before,
.hr-step-list::after {
  position: absolute;
  top: var(--hr-step-line-y);
  right: var(--hr-step-line-right);
  left: var(--hr-step-line-left);
  z-index: 0;
  height: 1px;
  border-radius: 999px;
  content: '';
  pointer-events: none;
}

.hr-step-list::before {
  background: linear-gradient(
    90deg,
    color-mix(in srgb, var(--hr-ink) 16%, transparent) 0 var(--hr-step-line-fade-start),
    transparent 100%
  );
}

.hr-step-list::after {
  display: none;
  height: 2px;
  background: linear-gradient(
    90deg,
    var(--hr-ink) 0 var(--hr-step-line-fade-start),
    transparent 100%
  );
  clip-path: inset(0 var(--hr-step-line-clip) 0 0);
  box-shadow: 0 0 10px color-mix(in srgb, var(--hr-ink) 28%, transparent);
  transform: translateY(-0.5px);
  will-change: clip-path;
}

.hr-step-list[data-scroll-motion]::after {
  display: block;
}

.hr-step-item {
  --hr-step-progress: 1;
  --hr-step-node-progress: 1;
  position: relative;
  z-index: 1;
  min-width: 0;
}

.hr-step-reveal {
  height: 260px;
}

.hr-step-list[data-scroll-motion] .hr-step-reveal {
  opacity: var(--hr-step-progress);
  transform: translate3d(0, calc((1 - var(--hr-step-progress)) * 24px), 0)
    scale(calc(0.985 + var(--hr-step-progress) * 0.015));
  transform-origin: left bottom;
  will-change: transform, opacity;
}

.hr-step-list[data-scroll-motion] .hr-step-item h3,
.hr-step-list[data-scroll-motion] .hr-step-item > p {
  opacity: var(--hr-step-progress);
  transform: translate3d(0, calc((1 - var(--hr-step-progress)) * 12px), 0);
  will-change: transform, opacity;
}

.hr-step-preview {
  position: relative;
  display: grid;
  width: 100%;
  height: 100%;
  min-width: 0;
  align-content: center;
  gap: 16px;
  overflow: hidden;
  border: 1px solid rgba(246, 248, 244, 0.72);
  border-radius: 26px;
  padding: 24px;
  color: rgba(20, 25, 24, 0.88);
  box-shadow: 0 20px 44px var(--hr-shadow-soft), inset 0 0 0 1px rgba(246, 248, 244, 0.28);
}

.preview-key { background: linear-gradient(145deg, rgba(255, 202, 173, 0.94), rgba(234, 177, 177, 0.78) 58%, rgba(167, 205, 224, 0.84)); }
.preview-config { background: linear-gradient(145deg, rgba(217, 207, 248, 0.94), rgba(194, 181, 231, 0.8) 56%, rgba(245, 180, 203, 0.82)); }
.preview-request { background: linear-gradient(145deg, rgba(186, 224, 211, 0.94), rgba(196, 217, 236, 0.84) 55%, rgba(245, 212, 150, 0.84)); }

.preview-key > span,
.preview-request > span {
  color: rgba(20, 25, 24, 0.55);
  font-size: 12px;
}

.preview-key > strong,
.preview-request > code,
.preview-config strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-key > strong {
  border: 1px solid rgba(246, 248, 244, 0.64);
  border-radius: 12px;
  padding: 15px;
  background: rgba(246, 248, 244, 0.4);
  box-shadow: 0 12px 24px rgba(81, 58, 72, 0.12);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.preview-key > em,
.preview-request > em {
  width: fit-content;
  border-radius: 999px;
  padding: 5px 9px;
  color: #205d50;
  background: rgba(218, 249, 232, 0.75);
  font-size: 10px;
  font-style: normal;
}

.preview-config {
  gap: 10px;
}

.preview-config > div {
  display: grid;
  grid-template-columns: 82px minmax(0, 1fr);
  gap: 12px;
  border: 1px solid rgba(246, 248, 244, 0.58);
  border-radius: 11px;
  padding: 11px;
  background: rgba(246, 248, 244, 0.34);
  font-size: 10px;
}

.preview-config span {
  color: rgba(20, 25, 24, 0.45);
}

.preview-config strong {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 9px;
}

.preview-request {
  grid-template-columns: 1fr auto;
  gap: 0;
}

.preview-request > span {
  display: flex;
  grid-column: 1 / -1;
  align-items: center;
  gap: 8px;
}

.preview-request > span::before {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #4bb58d;
  box-shadow: 0 0 0 4px rgba(75, 181, 141, 0.14);
  content: '';
}

.preview-request > strong {
  grid-column: 1 / -1;
  margin: 15px 0 20px;
  color: #146d59;
  font-size: 58px;
  letter-spacing: 0;
}

.preview-request > code {
  border-top: 1px solid rgba(20, 25, 24, 0.14);
  padding-top: 13px;
  color: rgba(20, 25, 24, 0.55);
  font-size: 10px;
}

.preview-request > em {
  width: auto;
  align-self: stretch;
  border-top: 1px solid rgba(20, 25, 24, 0.14);
  border-radius: 0;
  padding: 13px 0 0;
  color: rgba(20, 25, 24, 0.55);
  background: transparent;
  font-size: 10px;
}

.hr-step-node {
  position: relative;
  display: flex;
  height: 44px;
  align-items: center;
  margin-top: 24px;
}

.hr-step-index {
  display: grid;
  width: 44px;
  height: 44px;
  place-items: center;
  border-radius: 50%;
  color: #f4f5f2;
  background: #1c211f;
  box-shadow: 0 10px 24px rgba(23, 27, 25, 0.18);
  font-size: 11px;
  font-weight: 680;
  opacity: var(--hr-step-node-progress);
  transform: scale(calc(0.78 + var(--hr-step-node-progress) * 0.22));
  transform-origin: center;
  will-change: transform, opacity;
}

.hr-step-item h3 {
  margin: 24px 0 10px;
  font-size: 23px;
  font-weight: 740;
  letter-spacing: 0;
}

.hr-step-item > p {
  margin: 0;
  color: var(--hr-muted);
  font-size: 13px;
  line-height: 1.6;
}

@media (max-width: 900px) {
  .hr-steps-shell {
    width: min(calc(100% - 40px), 680px);
  }

  .hr-steps header {
    grid-template-columns: 1fr;
    gap: 24px;
  }

  .hr-steps h2 {
    max-width: 11ch;
    font-size: 2.5rem;
  }

  .hr-step-list {
    --hr-step-line-x: 22px;
    --hr-step-line-top: 0px;
    --hr-step-line-height: 1px;
    grid-template-columns: 1fr;
    gap: 46px;
  }

  .hr-step-list::before,
  .hr-step-list::after {
    top: var(--hr-step-line-top);
    right: auto;
    left: var(--hr-step-line-x);
    width: 1px;
    height: var(--hr-step-line-height);
    background: linear-gradient(
      180deg,
      color-mix(in srgb, var(--hr-ink) 16%, transparent) 0 var(--hr-step-line-fade-start),
      transparent 100%
    );
  }

  .hr-step-list::after {
    width: 2px;
    background: linear-gradient(
      180deg,
      var(--hr-ink) 0 var(--hr-step-line-fade-start),
      transparent 100%
    );
    clip-path: inset(0 0 var(--hr-step-line-clip) 0);
    transform: translateX(-0.5px);
  }

  .hr-step-reveal {
    height: 220px;
  }

  .hr-step-list[data-scroll-motion='mobile'] .hr-step-reveal,
  .hr-step-list[data-scroll-motion='mobile'] .hr-step-item h3,
  .hr-step-list[data-scroll-motion='mobile'] .hr-step-item > p {
    opacity: 1;
    transform: none;
  }

  .hr-step-item h3,
  .hr-step-item > p {
    margin-left: 64px;
  }

  .hr-step-preview {
    border-radius: 24px;
    padding: 22px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hr-step-list[data-scroll-motion] .hr-step-reveal,
  .hr-step-list[data-scroll-motion] .hr-step-item h3,
  .hr-step-list[data-scroll-motion] .hr-step-item > p,
  .hr-step-list[data-scroll-motion] .hr-step-index {
    opacity: 1;
    transform: none;
  }
}
</style>
