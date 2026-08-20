<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import BillingEstimator from '@/components/customer-console/billing/BillingEstimator.vue'
import CustomerFeatureHeader from '@/components/customer-console/CustomerFeatureHeader.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
</script>

<template>
  <div class="billing-panel">
    <CustomerFeatureHeader
      eyebrow="Cost telemetry / Billing"
      :title="t('native.billing.title')"
      :description="t('native.billing.description')"
      icon="calculator"
      :metrics="[
        { value: t('native.billing.metrics.unitValue'), label: t('native.billing.metrics.unitLabel') },
        { value: t('native.billing.metrics.partsValue'), label: t('native.billing.metrics.partsLabel') },
        { value: t('native.billing.metrics.recordValue'), label: t('native.billing.metrics.recordLabel') }
      ]"
    />

    <section class="billing-panel__formula" aria-labelledby="billing-formula-title">
      <div class="billing-panel__section-heading">
        <span class="billing-panel__index">01</span>
        <div>
          <h2 id="billing-formula-title">{{ t('native.billing.formula.title') }}</h2>
          <p>{{ t('native.billing.formula.description') }}</p>
        </div>
      </div>

      <div class="formula-strip">
        <div class="formula-strip__term">
          <span>{{ t('native.billing.formula.input') }}</span>
          <strong>{{ t('native.billing.formula.inputFormula') }}</strong>
        </div>
        <span class="formula-strip__operator">+</span>
        <div class="formula-strip__term">
          <span>{{ t('native.billing.formula.output') }}</span>
          <strong>{{ t('native.billing.formula.outputFormula') }}</strong>
        </div>
        <span class="formula-strip__operator">+</span>
        <div class="formula-strip__term">
          <span>{{ t('native.billing.formula.cache') }}</span>
          <strong>{{ t('native.billing.formula.cacheFormula') }}</strong>
        </div>
        <span class="formula-strip__operator">×</span>
        <div class="formula-strip__term formula-strip__term--accent">
          <span>{{ t('native.billing.formula.multiplier') }}</span>
          <strong>{{ t('native.billing.formula.total') }}</strong>
        </div>
      </div>
    </section>

    <section class="billing-panel__calculator" aria-labelledby="billing-calculator-title">
      <div class="billing-panel__section-heading">
        <span class="billing-panel__index">02</span>
        <div>
          <h2 id="billing-calculator-title">{{ t('native.billing.calculator.title') }}</h2>
          <p>{{ t('native.billing.calculator.description') }}</p>
        </div>
      </div>
      <BillingEstimator />
    </section>

    <section class="billing-panel__concepts" aria-labelledby="billing-concepts-title">
      <div class="billing-panel__section-heading">
        <span class="billing-panel__index">03</span>
        <div>
          <h2 id="billing-concepts-title">{{ t('native.billing.concepts.title') }}</h2>
          <p>{{ t('native.billing.concepts.description') }}</p>
        </div>
      </div>

      <div class="concept-grid">
        <article>
          <span>01 / CONTEXT</span>
          <h3>{{ t('native.billing.concepts.contextTitle') }}</h3>
          <p>{{ t('native.billing.concepts.contextBody') }}</p>
          <ul>
            <li>{{ t('native.billing.concepts.contextItem1') }}</li>
            <li>{{ t('native.billing.concepts.contextItem2') }}</li>
          </ul>
        </article>
        <article>
          <span>02 / CACHE</span>
          <h3>{{ t('native.billing.concepts.cacheTitle') }}</h3>
          <p>{{ t('native.billing.concepts.cacheBody') }}</p>
          <ul>
            <li>{{ t('native.billing.concepts.cacheItem1') }}</li>
            <li>{{ t('native.billing.concepts.cacheItem2') }}</li>
          </ul>
        </article>
        <article>
          <span>03 / AGENT LOOP</span>
          <h3>{{ t('native.billing.concepts.toolsTitle') }}</h3>
          <p>{{ t('native.billing.concepts.toolsBody') }}</p>
          <ul>
            <li>{{ t('native.billing.concepts.toolsItem1') }}</li>
            <li>{{ t('native.billing.concepts.toolsItem2') }}</li>
          </ul>
        </article>
      </div>
    </section>

    <section class="billing-panel__audit">
      <div>
        <span class="billing-panel__audit-kicker">VERIFY, THEN TRUST</span>
        <h2>{{ t('native.billing.reconcile.title') }}</h2>
        <p>{{ t('native.billing.reconcile.description') }}</p>
      </div>
      <div class="billing-panel__audit-points">
        <span><Icon name="check" size="sm" /> {{ t('native.billing.reconcile.model') }}</span>
        <span><Icon name="check" size="sm" /> {{ t('native.billing.reconcile.tokens') }}</span>
        <span><Icon name="check" size="sm" /> {{ t('native.billing.reconcile.multiplier') }}</span>
        <span><Icon name="check" size="sm" /> {{ t('native.billing.reconcile.charged') }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.billing-panel {
  width: 100%;
  min-width: 0;
  color: var(--customer-ink, #142126);
}

.billing-panel > * + * {
  margin-top: 22px;
}

.billing-panel__formula,
.billing-panel__calculator,
.billing-panel__concepts {
  padding: 22px 0 0;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.billing-panel__section-heading {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 16px;
}

.billing-panel__section-heading h2 {
  margin: 0;
  color: var(--customer-ink, #142126);
  font-size: 19px;
  font-weight: 680;
  letter-spacing: 0;
  line-height: 1.35;
}

.billing-panel__section-heading p {
  margin: 4px 0 0;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 13px;
  line-height: 1.65;
}

.billing-panel__index {
  display: inline-grid;
  flex: 0 0 30px;
  width: 30px;
  height: 24px;
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 10px;
  font-weight: 700;
  place-items: center;
  background: var(--customer-cyan-soft, rgba(10, 127, 144, 0.1));
  border-left: 2px solid var(--customer-cyan, #0a7f90);
}

.formula-strip {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) auto minmax(140px, 1fr) auto minmax(140px, 1fr) auto minmax(140px, 1fr);
  gap: 10px;
  align-items: stretch;
  padding: 14px;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.formula-strip__term {
  min-width: 0;
  padding: 12px;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 5px;
}

.formula-strip__term span,
.formula-strip__term strong {
  display: block;
}

.formula-strip__term span {
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-family: var(--customer-mono, monospace);
  font-size: 10px;
  line-height: 1.35;
  text-transform: uppercase;
}

.formula-strip__term strong {
  margin-top: 5px;
  color: var(--customer-ink, #142126);
  font-size: 12px;
  font-weight: 650;
  line-height: 1.5;
}

.formula-strip__term--accent {
  background: var(--customer-copper-soft, rgba(156, 100, 31, 0.11));
  border-color: color-mix(in srgb, var(--customer-copper, #9c641f) 35%, transparent);
}

.formula-strip__term--accent span {
  color: var(--customer-copper, #9c641f);
}

.formula-strip__operator {
  display: grid;
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 16px;
  font-weight: 700;
  place-items: center;
}

.concept-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.concept-grid article {
  min-width: 0;
  padding: 18px;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.concept-grid article > span {
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
  font-weight: 700;
}

.concept-grid article:nth-child(2) > span {
  color: var(--customer-copper, #9c641f);
}

.concept-grid h3 {
  margin: 8px 0 6px;
  color: var(--customer-ink, #142126);
  font-size: 15px;
  font-weight: 680;
}

.concept-grid p,
.concept-grid li {
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 12px;
  line-height: 1.65;
}

.concept-grid p {
  margin: 0;
}

.concept-grid ul {
  padding: 10px 0 0 17px;
  margin: 12px 0 0;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.billing-panel__audit {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(280px, 0.7fr);
  gap: 28px;
  align-items: center;
  padding: 22px;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border-left: 3px solid var(--customer-copper, #9c641f);
  border-radius: 0 7px 7px 0;
}

.billing-panel__audit-kicker {
  color: var(--customer-copper, #9c641f);
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
  font-weight: 700;
}

.billing-panel__audit h2 {
  margin: 5px 0 6px;
  color: var(--customer-ink, #142126);
  font-size: 18px;
}

.billing-panel__audit p {
  max-width: 720px;
  margin: 0;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 12px;
  line-height: 1.7;
}

.billing-panel__audit-points {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.billing-panel__audit-points span {
  display: flex;
  min-width: 0;
  gap: 7px;
  align-items: center;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 11px;
}

.billing-panel__audit-points svg {
  flex: 0 0 auto;
  color: var(--customer-cyan, #0a7f90);
}

@media (max-width: 1100px) {
  .formula-strip {
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  }
}

@media (max-width: 900px) {
  .concept-grid {
    grid-template-columns: 1fr;
  }

  .billing-panel__audit {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 620px) {
  .billing-panel > * + * {
    margin-top: 18px;
  }

  .formula-strip {
    grid-template-columns: 1fr;
  }

  .formula-strip__operator {
    min-height: 18px;
  }

  .billing-panel__audit {
    padding: 18px;
  }

  .billing-panel__audit-points {
    grid-template-columns: 1fr;
  }
}
</style>
