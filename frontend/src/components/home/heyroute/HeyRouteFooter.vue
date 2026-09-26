<script setup lang="ts">
import type { HeyRouteCopy } from './heyRouteCopy'

interface Props {
  copy: HeyRouteCopy['footer']
  siteName: string
  siteLogo: string
  docUrl: string
  modelPlazaEnabled: boolean
  dashboardPath: string
}

defineProps<Props>()

const emit = defineEmits<{
  openCommunity: []
  openSupport: []
}>()

const currentYear = new Date().getFullYear()

function scrollToFaq(): void {
  document.querySelector('#hr-faq')?.scrollIntoView({ behavior: 'smooth' })
}
</script>

<template>
  <footer class="hr-footer">
    <div class="hr-section-shell">
      <div class="hr-footer-brand">
        <img :src="siteLogo || '/logo.svg'" :alt="siteName" />
        <strong>{{ siteName }}</strong>
      </div>
      <h2>{{ copy.statement }}</h2>
      <p class="hr-footer-note">{{ copy.note }}</p>

      <div class="hr-footer-grid">
        <div>
          <h3>{{ copy.product }}</h3>
          <router-link :to="dashboardPath">{{ copy.console }}</router-link>
          <router-link v-if="modelPlazaEnabled" to="/model-plaza">{{ copy.models }}</router-link>
        </div>
        <div>
          <h3>{{ copy.resources }}</h3>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ copy.docs }}</a>
          <button type="button" @click="scrollToFaq">{{ copy.faq }}</button>
        </div>
        <div>
          <h3>{{ copy.developer }}</h3>
          <router-link to="/key-usage">{{ copy.usage }}</router-link>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ copy.api }}</a>
        </div>
        <div>
          <h3>{{ copy.legal }}</h3>
          <router-link to="/home">{{ copy.about }}</router-link>
          <router-link to="/legal/privacy">{{ copy.privacy }}</router-link>
          <router-link to="/legal/terms">{{ copy.terms }}</router-link>
        </div>
        <div>
          <h3>{{ copy.contact }}</h3>
          <button type="button" @click="emit('openSupport')">{{ copy.ticket }}</button>
          <button type="button" @click="emit('openCommunity')">{{ copy.community }}</button>
        </div>
      </div>

      <div class="hr-footer-legal">
        <span>© {{ currentYear }} {{ siteName }}</span>
        <span>{{ copy.status }}</span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.hr-footer {
  padding: 86px 0 32px;
  color: var(--hr-ink);
  background: linear-gradient(180deg, transparent, var(--hr-footer-bg) 24%, var(--hr-footer-bg));
}

.hr-footer-brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.hr-footer-brand img {
  width: 38px;
  height: 38px;
  border-radius: 0;
  filter: drop-shadow(0 7px 10px rgba(39, 25, 19, 0.16));
}

.hr-footer-brand strong {
  font-size: 20px;
  font-weight: 740;
}

.hr-footer h2 {
  margin: 20px 0 12px;
  font-size: 2.7rem;
  font-weight: 720;
  letter-spacing: 0;
  line-height: 1.05;
}

.hr-footer-note {
  max-width: 760px;
  margin: 0 0 54px;
  color: var(--hr-muted);
  font-size: 13px;
  line-height: 1.7;
}

.hr-footer-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 38px;
}

.hr-footer-grid > div {
  display: grid;
  align-content: start;
  gap: 11px;
}

.hr-footer-grid h3 {
  margin: 0 0 7px;
  color: var(--hr-muted);
  font-size: 10px;
  font-weight: 680;
  text-transform: none;
}

.hr-footer-grid a,
.hr-footer-grid button {
  width: fit-content;
  border: 0;
  padding: 0;
  color: var(--hr-ink);
  background: transparent;
  font: inherit;
  font-size: 13px;
  text-decoration: none;
  cursor: pointer;
}

.hr-footer-grid a:hover,
.hr-footer-grid button:hover {
  color: var(--hr-muted);
}

.hr-footer-legal {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  margin-top: 60px;
  color: var(--hr-muted);
  font-size: 10px;
}

.hr-footer-grid a:focus-visible,
.hr-footer-grid button:focus-visible {
  outline: 2px solid var(--hr-focus);
  outline-offset: 3px;
}

@media (max-width: 760px) {
  .hr-footer h2 {
    font-size: 2rem;
  }

  .hr-footer-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 34px 22px;
  }

  .hr-footer-legal {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
