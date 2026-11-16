<script setup lang="ts">
import { shallowRef } from 'vue'

import Icon from '@/components/icons/Icon.vue'
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['faq']
  docUrl: string
}

defineProps<Props>()

const openIndex = shallowRef<number | null>(0)

function toggle(index: number): void {
  openIndex.value = openIndex.value === index ? null : index
}
</script>

<template>
  <section id="hr-faq" class="hr-faq hr-content-stage" data-hr-section>
    <div class="hr-section-shell hr-faq-shell">
      <aside>
        <h2>{{ copy.title }}</h2>
        <p>{{ copy.description }}</p>
        <div class="hr-faq-actions">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
            <Icon name="book" size="sm" />{{ copy.docs }}
          </a>
          <router-link to="/login"><Icon name="chat" size="sm" />{{ copy.support }}</router-link>
        </div>
      </aside>

      <div class="hr-faq-list">
        <article v-for="(item, index) in copy.items" :key="item.question" :class="{ open: openIndex === index }">
          <h3>
            <button
              type="button"
              :aria-expanded="openIndex === index"
              :aria-controls="`hr-faq-answer-${index}`"
              @click="toggle(index)"
            >
              <span class="hr-faq-number">{{ String(index + 1).padStart(2, '0') }}</span>
              <span>{{ item.question }}</span>
              <Icon :name="openIndex === index ? 'x' : 'plus'" size="sm" />
            </button>
          </h3>
          <div v-show="openIndex === index" :id="`hr-faq-answer-${index}`" class="hr-faq-answer">
            <p>{{ item.answer }}</p>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.hr-faq {
  position: relative;
  padding: 120px 0;
  background:
    radial-gradient(circle at 7% 78%, rgba(233, 150, 104, 0.15), transparent 28%),
    radial-gradient(circle at 92% 16%, rgba(113, 160, 205, 0.15), transparent 30%),
    var(--hr-bg);
}

.hr-faq-shell {
  display: grid;
  grid-template-columns: minmax(320px, 0.78fr) minmax(0, 1.22fr);
  gap: clamp(60px, 8vw, 120px);
  align-items: start;
}

.hr-faq aside {
  position: sticky;
  top: 120px;
}

.hr-faq h2 {
  max-width: 11ch;
  margin: 0;
  font-size: 4.7rem;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 0.96;
  text-wrap: balance;
}

.hr-faq aside > p {
  max-width: 460px;
  margin: 24px 0 0;
  color: var(--hr-muted);
  font-size: 15px;
  line-height: 1.65;
}

.hr-faq-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 30px;
}

.hr-faq-actions a {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--hr-line);
  border-radius: 6px;
  padding: 0 14px;
  color: var(--hr-ink);
  background: var(--hr-panel-soft);
  font-size: 12px;
  font-weight: 650;
  text-decoration: none;
}

.hr-faq-list {
  border-top: 1px solid var(--hr-line-strong);
}

.hr-faq-list article {
  border-bottom: 1px solid var(--hr-line);
}

.hr-faq-list h3 {
  margin: 0;
}

.hr-faq-list button {
  display: grid;
  width: 100%;
  min-height: 86px;
  grid-template-columns: 42px minmax(0, 1fr) 24px;
  gap: 14px;
  align-items: center;
  border: 0;
  padding: 18px 0;
  color: var(--hr-ink);
  background: transparent;
  font: inherit;
  font-size: 17px;
  font-weight: 680;
  text-align: left;
  cursor: pointer;
}

.hr-faq-number {
  color: var(--hr-muted);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.hr-faq-answer {
  padding: 0 38px 26px 56px;
}

.hr-faq-answer p {
  max-width: 700px;
  margin: 0;
  color: var(--hr-muted);
  font-size: 14px;
  line-height: 1.75;
}

.hr-faq-actions a:focus-visible,
.hr-faq-list button:focus-visible {
  outline: 2px solid var(--hr-focus);
  outline-offset: 3px;
}

@media (max-width: 900px) {
  .hr-faq {
    padding: 84px 0;
  }

  .hr-faq-shell {
    grid-template-columns: 1fr;
    gap: 54px;
  }

  .hr-faq aside {
    position: static;
  }

  .hr-faq h2 {
    font-size: 2.5rem;
  }

  .hr-faq-list button {
    min-height: 76px;
    font-size: 15px;
  }

  .hr-faq-answer {
    padding-left: 56px;
  }
}
</style>
