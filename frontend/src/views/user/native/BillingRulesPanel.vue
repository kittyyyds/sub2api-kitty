<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getModelPlaza } from '@/api/modelPlaza'
import BillingEstimator from '@/components/customer-console/billing/BillingEstimator.vue'
import CustomerFeatureHeader from '@/components/customer-console/CustomerFeatureHeader.vue'
import Icon from '@/components/icons/Icon.vue'
const { t } = useI18n()
const formulaOpen = ref(true), trapsOpen = ref(true), competitorMultiplier = ref(3)
const tokenInput = ref(1_000_000), tokenOutput = ref(1_000_000), tokenMultiplier = ref(0.8), cacheEnabled = ref(true)
const inputPrice = ref(5)
const outputPrice = ref(15)
const cacheWritePrice = ref(1)
const cacheReadPrice = ref(0.5)
const tokenDots = Array.from({ length: 12 }, (_, index) => ({
  id: index,
  top: `${14 + (index * 23) % 76}%`,
  size: `${5 + (index % 4) * 2}px`,
  delay: `${-(index * 0.72).toFixed(2)}s`,
  duration: `${5.8 + (index % 5) * 0.7}s`,
}))
const copy = computed(() => ({
  eyebrow: t('native.billing.showcase.eyebrow'),
  heroTitle: t('native.billing.showcase.heroTitle'),
  heroBody: t('native.billing.showcase.heroBody'),
  viewCalc: t('native.billing.showcase.viewCalculator'),
  formula: t('native.billing.showcase.formula'),
  formulaBody: t('native.billing.showcase.formulaBody'),
  input: t('native.billing.showcase.input'),
  output: t('native.billing.showcase.output'),
  cache: t('native.billing.showcase.cache'),
  multiplier: t('native.billing.showcase.multiplier'),
  result: t('native.billing.showcase.result'),
  basis: t('native.billing.showcase.basis'),
  insights: [
    { title: t('native.billing.showcase.insights.priceTitle'), body: t('native.billing.showcase.insights.priceBody') },
    { title: t('native.billing.showcase.insights.rateTitle'), body: t('native.billing.showcase.insights.rateBody') },
    { title: t('native.billing.showcase.insights.traceTitle'), body: t('native.billing.showcase.insights.traceBody') },
  ],
  transparencyTitle: t('native.billing.showcase.transparencyTitle'),
  transparencyBody: t('native.billing.showcase.transparencyBody'),
  riskLabels: ['high', 'misleading', 'hidden', 'trap'].map((key) => t(`native.billing.showcase.riskLabels.${key}`)),
  traps: ['longContext', 'cacheRates', 'toolResults', 'peakRate'].map((key) => t(`native.billing.showcase.traps.${key}`)),
  trapCardBody: t('native.billing.showcase.trapCardBody'),
  compare: t('native.billing.showcase.compare'),
  compareBody: t('native.billing.showcase.compareBody'),
  competitorMultiplier: t('native.billing.showcase.competitorMultiplier'),
  yourCost: t('native.billing.showcase.yourCost'),
  competitorCost: t('native.billing.showcase.competitorCost'),
  save: t('native.billing.showcase.saving'),
  tokenLab: t('native.billing.showcase.tokenTitle'),
  tokenBody: t('native.billing.showcase.tokenBody'),
  cacheExample: t('native.billing.showcase.cacheExample'),
  cacheHint: t('native.billing.showcase.cacheHint'),
  advanced: t('native.billing.showcase.advanced.title'),
  advancedSummary: t('native.billing.showcase.advanced.summary'),
  advancedCards: [
    {
      title: t('native.billing.showcase.advanced.cacheTitle'),
      body: t('native.billing.showcase.advanced.cacheBody'),
      items: [t('native.billing.showcase.advanced.cacheItem1'), t('native.billing.showcase.advanced.cacheItem2')],
    },
    {
      title: t('native.billing.showcase.advanced.requestTitle'),
      body: t('native.billing.showcase.advanced.requestBody'),
      items: [t('native.billing.showcase.advanced.requestItem1'), t('native.billing.showcase.advanced.requestItem2')],
    },
    {
      title: t('native.billing.showcase.advanced.contextTitle'),
      body: t('native.billing.showcase.advanced.contextBody'),
      items: [t('native.billing.showcase.advanced.contextItem1'), t('native.billing.showcase.advanced.contextItem2')],
    },
  ],
  promise: t('native.billing.showcase.promise'),
  promiseBody: t('native.billing.showcase.promiseBody'),
  cta: t('native.billing.showcase.usageDetails'),
}))
function scrollToCalculator(){ document.getElementById('billing-calculator')?.scrollIntoView({behavior:'smooth',block:'start'}) }
const tokenTotal = computed(() => tokenInput.value + tokenOutput.value)
const inputShare = computed(() => tokenTotal.value ? Math.round(tokenInput.value / tokenTotal.value * 100) : 0)
const cacheCost = computed(() => cacheEnabled.value ? ((tokenInput.value * 0.2) / 1_000_000 * cacheWritePrice.value + (tokenInput.value * 0.55) / 1_000_000 * cacheReadPrice.value) : 0)
const visualCost = computed(() => (((tokenInput.value / 1_000_000) * inputPrice.value + (tokenOutput.value / 1_000_000) * outputPrice.value) + cacheCost.value) * tokenMultiplier.value)
const compareBase = computed(() => ((tokenInput.value / 1_000_000) * inputPrice.value + (tokenOutput.value / 1_000_000) * outputPrice.value + cacheCost.value) )
const compareMine = computed(() => compareBase.value * tokenMultiplier.value)
const compareReference = computed(() => compareBase.value * competitorMultiplier.value)
const saving = computed(() => Math.max(0, compareReference.value - compareMine.value))
const money = (value:number) => `¥${value.toFixed(2)}`
onMounted(async () => {
  if (import.meta.env.MODE === 'test') return
  try {
    const plaza = await getModelPlaza()
    const model = plaza.groups.flatMap((group) => group.models).find((entry) => entry.name.toLowerCase() === 'gpt-5.5')
    const pricing = model?.official_pricing ?? model?.pricing
    if (pricing) {
      inputPrice.value = Number(pricing.input_price ?? pricing.input_price ?? 0) * 1e6
      outputPrice.value = Number(pricing.output_price ?? 0) * 1e6
      cacheWritePrice.value = Number(pricing.cache_write_price ?? 0) * 1e6
      cacheReadPrice.value = Number(pricing.cache_read_price ?? 0) * 1e6
    }
  } catch { /* the live estimator remains the source of truth when the catalog is unavailable */ }
})
</script>
<template>
  <div class="billing-panel">
    <CustomerFeatureHeader eyebrow="Cost telemetry / Billing" :title="t('native.billing.title')" :description="t('native.billing.description')" icon="calculator" :metrics="[{ value: t('native.billing.metrics.unitValue'), label: t('native.billing.metrics.unitLabel') }, { value: t('native.billing.metrics.partsValue'), label: t('native.billing.metrics.partsLabel') }, { value: t('native.billing.metrics.recordValue'), label: t('native.billing.metrics.recordLabel') }]" />
    <section class="billing-hero"><div class="billing-hero__copy"><span class="billing-kicker">{{ copy.eyebrow }}</span><h1>{{ copy.heroTitle }}</h1><p>{{ copy.heroBody }}</p><button class="billing-hero__cta" type="button" @click="scrollToCalculator"><Icon name="calculator" size="sm" /> {{ copy.viewCalc }}</button></div><div class="billing-hero__signal" aria-hidden="true"><span>INPUT</span><b>+</b><span>OUTPUT</span><b>×</b><strong>RATE</strong></div></section>
    <section class="billing-section billing-formula" aria-labelledby="billing-formula-title"><button class="section-toggle" type="button" :aria-expanded="formulaOpen" @click="formulaOpen = !formulaOpen"><span class="section-number">01</span><span><strong id="billing-formula-title">{{ copy.formula }}</strong><small>{{ copy.formulaBody }}</small></span><Icon name="chevronDown" size="sm" :class="formulaOpen && 'is-open'" /></button><Transition name="collapse"><div v-if="formulaOpen" class="formula-grid"><div><span>{{ copy.input }}</span><strong>Token × input price</strong></div><i>+</i><div><span>{{ copy.output }}</span><strong>Token × output price</strong></div><i>+</i><div><span>{{ copy.cache }}</span><strong>Read / write rules</strong></div><i>×</i><div class="formula-grid__accent"><span>{{ copy.multiplier }}</span><strong>{{ tokenMultiplier.toFixed(2) }}x</strong></div><i>=</i><div class="formula-grid__total"><span>{{ copy.result }}</span><strong>{{ copy.basis }}</strong></div></div></Transition></section>
    <section class="insight-grid" aria-label="billing principles"><article v-for="(insight, index) in copy.insights" :key="insight.title"><span>0{{ index + 1 }} / {{ ['PRICE', 'RATE', 'TRACE'][index] }}</span><h3>{{ insight.title }}</h3><p>{{ insight.body }}</p></article></section>
    <section class="billing-section transparency-section"><div class="section-heading"><span class="section-number">01+</span><div><h2>{{ copy.transparencyTitle }}</h2><p>{{ copy.transparencyBody }}</p></div></div><div class="trap-cards"><article v-for="(trap, index) in copy.traps" :key="trap"><span>{{ copy.riskLabels[index] }}</span><h3>{{ trap }}</h3><p>{{ copy.trapCardBody }}</p></article></div></section>
    <section class="billing-section compare-section"><div class="section-heading"><span class="section-number">02</span><div><h2>{{ copy.compare }}</h2><p>{{ copy.compareBody }}</p></div></div><div class="compare-layout"><div class="compare-slider"><label>{{ copy.competitorMultiplier }} <b>{{ competitorMultiplier.toFixed(1) }}x</b></label><input v-model.number="competitorMultiplier" type="range" min="1" max="8" step="0.1" /></div><div class="compare-results"><div><span>{{ copy.yourCost }} · {{ tokenMultiplier.toFixed(2) }}x</span><strong>{{ money(compareMine) }}</strong></div><div><span>{{ copy.competitorCost }} · {{ competitorMultiplier.toFixed(1) }}x</span><strong>{{ money(compareReference) }}</strong></div><em>{{ copy.save }} {{ money(saving) }}</em></div></div></section>
    <section class="billing-section token-section"><div class="section-heading"><span class="section-number">03</span><div><h2>{{ copy.tokenLab }}</h2><p>{{ copy.tokenBody }}</p></div></div><div class="token-layout"><div class="token-controls"><label>{{ copy.input }} <b>{{ tokenInput.toLocaleString() }}</b><input v-model.number="tokenInput" type="range" min="1000" max="1000000" step="1000" /></label><label>{{ copy.output }} <b>{{ tokenOutput.toLocaleString() }}</b><input v-model.number="tokenOutput" type="range" min="1000" max="1000000" step="1000" /></label><label>{{ copy.multiplier }} <b>{{ tokenMultiplier.toFixed(2) }}x</b><input v-model.number="tokenMultiplier" type="range" min="0" max="1" step="0.01" /></label><label class="cache-toggle"><input v-model="cacheEnabled" type="checkbox" /><span>{{ copy.cacheExample }}</span></label></div><div class="token-stage"><div class="token-stage__bar"><i :style="{width:`${inputShare}%`}"></i><b :style="{width:`${100 - inputShare}%`}"></b></div><div class="token-stage__flow" aria-hidden="true"><i v-for="dot in tokenDots" :key="dot.id" :style="{ top: dot.top, width: dot.size, height: dot.size, animationDelay: dot.delay, animationDuration: dot.duration }"></i></div><div class="token-stage__legend"><span><i class="dot dot--input"></i>{{ copy.input }} {{ inputShare }}%</span><span><i class="dot dot--output"></i>{{ copy.output }} {{ 100 - inputShare }}%</span></div><strong>{{ money(visualCost) }}</strong><small>{{ cacheEnabled ? copy.cacheHint : copy.basis }}</small></div></div></section>
    <section id="billing-calculator" class="billing-section calculator-section" aria-labelledby="billing-calculator-title"><div class="section-heading"><span class="section-number">04</span><div><h2 id="billing-calculator-title">{{ t('native.billing.calculator.title') }}</h2><p>{{ t('native.billing.calculator.description') }}</p></div></div><BillingEstimator /></section>
    <section class="billing-section traps-section"><button class="section-toggle" type="button" :aria-expanded="trapsOpen" @click="trapsOpen = !trapsOpen"><span class="section-number">05</span><span><strong>{{ copy.advanced }}</strong><small>{{ copy.advancedSummary }}</small></span><Icon name="chevronDown" size="sm" :class="trapsOpen && 'is-open'" /></button><Transition name="collapse"><div v-if="trapsOpen" class="advanced-cards"><article v-for="card in copy.advancedCards" :key="card.title"><h3>{{ card.title }}</h3><p>{{ card.body }}</p><ul><li v-for="item in card.items" :key="item">{{ item }}</li></ul></article></div></Transition></section>
    <section class="billing-promise"><div><span class="billing-kicker">VERIFY, THEN TRUST</span><h2>{{ copy.promise }}</h2><p>{{ copy.promiseBody }}</p></div><button type="button" @click="scrollToCalculator">{{ copy.cta }} <Icon name="arrowRight" size="sm" /></button></section>
  </div>
</template>
<style scoped>
.billing-panel{width:100%;min-width:0;color:var(--customer-ink,#142126)}.billing-panel>*+*{margin-top:24px}.billing-hero{display:flex;justify-content:space-between;gap:26px;padding:28px 0 30px;border-bottom:1px solid var(--customer-line,rgba(20,33,38,.12))}.billing-hero__copy{max-width:680px}.billing-kicker{color:var(--customer-cyan,#0a7f90);font:700 10px var(--customer-mono,monospace);letter-spacing:.08em;text-transform:uppercase}.billing-hero h1{margin:10px 0 8px;font-size:clamp(28px,4vw,46px);line-height:1.1;letter-spacing:0}.billing-hero p{max-width:610px;margin:0;color:var(--customer-muted,rgba(20,33,38,.66));font-size:14px;line-height:1.7}.billing-hero__cta,.billing-promise button{display:inline-flex;align-items:center;gap:8px;margin-top:20px;padding:10px 14px;color:#fff;font:650 12px inherit;background:var(--customer-cyan,#0a7f90);border:0;border-radius:5px;cursor:pointer}.billing-hero__signal{display:grid;grid-template-columns:auto auto auto auto auto;align-content:center;gap:10px;color:var(--customer-faint,rgba(20,33,38,.48));font:700 10px var(--customer-mono,monospace)}.billing-hero__signal b{color:var(--customer-copper,#9c641f);font-size:18px}.billing-hero__signal strong{color:var(--customer-copper,#9c641f)}.billing-section{padding-top:22px;border-top:1px solid var(--customer-line,rgba(20,33,38,.12))}.section-heading,.section-toggle{display:flex;align-items:flex-start;gap:12px}.section-heading{margin-bottom:16px}.section-heading h2,.section-toggle strong{margin:0;font-size:19px;font-weight:680}.section-heading p,.section-toggle small{display:block;margin:4px 0 0;color:var(--customer-muted,rgba(20,33,38,.66));font-size:13px;line-height:1.6}.section-number{display:inline-grid;flex:0 0 30px;width:30px;height:24px;color:var(--customer-cyan,#0a7f90);font:700 10px var(--customer-mono,monospace);place-items:center;background:var(--customer-cyan-soft,rgba(10,127,144,.1));border-left:2px solid currentColor}.section-toggle{width:100%;padding:0;text-align:left;color:inherit;background:transparent;border:0;cursor:pointer}.section-toggle>span:nth-child(2){flex:1}.section-toggle>svg{margin-top:5px;transition:transform .18s}.section-toggle>svg.is-open{transform:rotate(180deg)}.formula-grid{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr) auto minmax(0,1fr) auto minmax(100px,.8fr) auto minmax(150px,1.2fr);gap:9px;align-items:center;padding:14px;background:var(--customer-surface,rgba(248,251,252,.82));border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:7px}.formula-grid>div{padding:12px;background:var(--customer-surface-soft,rgba(228,237,240,.82));border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:5px}.formula-grid span{display:block;color:var(--customer-faint,rgba(20,33,38,.48));font:10px var(--customer-mono,monospace);text-transform:uppercase}.formula-grid strong{display:block;margin-top:5px;font-size:12px;line-height:1.4}.formula-grid i{font-style:normal;color:var(--customer-cyan,#0a7f90);font-weight:700}.formula-grid__accent{background:var(--customer-copper-soft,rgba(156,100,31,.11))!important}.formula-grid__accent strong,.formula-grid__accent span{color:var(--customer-copper,#9c641f)}.formula-grid__total{border-color:var(--customer-cyan,#0a7f90)!important}.insight-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.insight-grid article{padding:18px;background:var(--customer-surface,rgba(248,251,252,.82));border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:7px}.insight-grid span{color:var(--customer-cyan,#0a7f90);font:700 9px var(--customer-mono,monospace)}.insight-grid article:nth-child(2) span{color:var(--customer-copper,#9c641f)}.insight-grid h3{margin:8px 0 5px;font-size:15px}.insight-grid p{margin:0;color:var(--customer-muted,rgba(20,33,38,.66));font-size:12px;line-height:1.65}.compare-layout,.token-layout{display:grid;grid-template-columns:minmax(0,1fr) minmax(260px,.75fr);gap:18px;align-items:center;padding:18px;background:var(--customer-surface,rgba(248,251,252,.82));border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:7px}.compare-slider label,.token-controls label{display:block;color:var(--customer-muted,rgba(20,33,38,.66));font-size:12px}.compare-slider b,.token-controls b{float:right;color:var(--customer-copper,#9c641f);font-family:var(--customer-mono,monospace)}input[type=range]{display:block;width:100%;margin-top:16px;accent-color:var(--customer-cyan,#0a7f90)}.compare-results{display:grid;grid-template-columns:1fr 1fr;gap:10px}.compare-results div{padding:12px;background:var(--customer-surface-soft,rgba(228,237,240,.82));border-radius:5px}.compare-results span{display:block;color:var(--customer-faint,rgba(20,33,38,.48));font-size:10px}.compare-results strong{display:block;margin-top:5px;font:700 18px var(--customer-mono,monospace)}.compare-results em{grid-column:1/-1;color:var(--customer-copper,#9c641f);font-size:11px;font-style:normal}.token-controls{display:grid;gap:18px}.token-stage{text-align:center}.token-stage__bar{display:flex;height:22px;overflow:hidden;background:var(--customer-surface-soft,rgba(228,237,240,.82));border-radius:4px}.token-stage__bar i{display:block;background:var(--customer-cyan,#0a7f90);transition:width .25s}.token-stage__legend{display:flex;justify-content:space-between;margin-top:9px;color:var(--customer-faint,rgba(20,33,38,.48));font-size:10px}.dot{display:inline-block;width:7px;height:7px;margin-right:4px;border-radius:50%;background:var(--customer-cyan,#0a7f90)}.dot--output{background:var(--customer-copper,#9c641f)}.token-stage>strong{display:block;margin-top:16px;font:700 26px var(--customer-mono,monospace)}.token-stage>small{color:var(--customer-faint,rgba(20,33,38,.48));font-size:10px}.traps-section{padding-bottom:4px}.trap-list{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;padding:14px 0 0 42px;margin:0;list-style:none}.trap-list li{display:flex;gap:7px;align-items:center;color:var(--customer-muted,rgba(20,33,38,.66));font-size:12px}.trap-list svg{color:var(--customer-copper,#9c641f);flex:0 0 auto}.billing-promise{display:flex;justify-content:space-between;gap:20px;align-items:center;padding:22px;background:var(--customer-surface-soft,rgba(228,237,240,.82));border-left:3px solid var(--customer-copper,#9c641f);border-radius:0 7px 7px 0}.billing-promise h2{margin:5px 0 6px;font-size:19px}.billing-promise p{max-width:680px;margin:0;color:var(--customer-muted,rgba(20,33,38,.66));font-size:12px;line-height:1.6}.billing-promise button{margin:0;white-space:nowrap}.collapse-enter-active,.collapse-leave-active{transition:opacity .18s,transform .18s}.collapse-enter-from,.collapse-leave-to{opacity:0;transform:translateY(-5px)}
@media (max-width:900px){.billing-hero__signal{display:none}.formula-grid{grid-template-columns:repeat(3,minmax(0,1fr))}.formula-grid i{display:none}.insight-grid{grid-template-columns:1fr}.compare-layout,.token-layout{grid-template-columns:1fr}.billing-promise{align-items:flex-start;flex-direction:column}.billing-promise button{margin-top:4px}.trap-list{grid-template-columns:1fr}}@media (max-width:620px){.billing-hero{padding-top:16px}.billing-hero h1{font-size:30px}.formula-grid{grid-template-columns:1fr}.formula-grid>div{padding:10px}.compare-results{grid-template-columns:1fr}.compare-results em{grid-column:auto}.billing-panel>*+*{margin-top:18px}}
.trap-cards,.advanced-cards{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.trap-cards article,.advanced-cards article{padding:14px;background:var(--customer-surface,rgba(248,251,252,.82));border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:7px}.trap-cards span{display:inline-block;padding:3px 6px;color:var(--customer-copper,#9c641f);font-size:10px;font-weight:700;background:var(--customer-copper-soft,rgba(156,100,31,.11));border-radius:999px}.trap-cards h3,.advanced-cards h3{margin:8px 0 5px;font-size:14px}.trap-cards p,.advanced-cards p,.advanced-cards li{margin:0;color:var(--customer-muted,rgba(20,33,38,.66));font-size:11px;line-height:1.6}.advanced-cards{grid-template-columns:repeat(3,minmax(0,1fr));padding:14px 0 0 42px}.advanced-cards ul{padding-left:16px;margin:9px 0 0;border-top:1px solid var(--customer-line,rgba(20,33,38,.12));padding-top:8px}.cache-toggle{display:flex!important;gap:8px;align-items:center}.cache-toggle input{width:15px!important;height:15px!important;accent-color:var(--customer-cyan,#0a7f90)}
@media (max-width:900px){.trap-cards{grid-template-columns:repeat(2,minmax(0,1fr))}.advanced-cards{grid-template-columns:1fr;padding-left:42px}}@media (max-width:620px){.trap-cards{grid-template-columns:1fr}.advanced-cards{padding-left:0}}
.billing-panel{position:relative;isolation:isolate;width:calc(100% + 48px);min-height:100%;padding:24px;background:var(--customer-page-bg,#f2f7f8);overflow:hidden}.billing-panel::before{position:absolute;inset:0;z-index:-1;pointer-events:none;content:'';opacity:.28;background-image:linear-gradient(rgba(10,127,144,.08) 1px,transparent 1px),linear-gradient(90deg,rgba(10,127,144,.08) 1px,transparent 1px);background-size:32px 32px}.billing-panel>*{position:relative}.billing-hero{padding:clamp(34px,6vw,74px) clamp(22px,5vw,64px);background:var(--customer-surface,rgba(248,251,252,.72));border:1px solid var(--customer-line,rgba(20,33,38,.14));border-radius:14px;box-shadow:0 22px 55px rgba(25,47,55,.12)}.billing-hero h1{font-size:clamp(34px,5vw,62px);font-weight:760;letter-spacing:.01em}.billing-section{padding:30px 0 0}.section-heading h2,.section-toggle strong{font-size:clamp(21px,2.4vw,30px);font-weight:730;letter-spacing:.01em}.formula-grid,.compare-layout,.token-layout,.insight-grid article,.trap-cards article,.advanced-cards article,.billing-promise{background:color-mix(in srgb,var(--customer-surface,#f8fbfc) 76%,transparent);-webkit-backdrop-filter:blur(14px);backdrop-filter:blur(14px);box-shadow:0 12px 32px rgba(25,47,55,.08)}.formula-grid>div,.compare-results div{background:color-mix(in srgb,var(--customer-surface-soft,#e4edf0) 72%,transparent)}.billing-hero__cta,.billing-promise button{box-shadow:0 8px 20px color-mix(in srgb,var(--customer-cyan,#0a7f90) 25%,transparent);transition:transform .16s ease,box-shadow .16s ease}.billing-hero__cta:hover,.billing-promise button:hover{transform:translateY(-2px);box-shadow:0 12px 26px color-mix(in srgb,var(--customer-cyan,#0a7f90) 32%,transparent)}.token-stage{position:relative;overflow:hidden;padding:18px;border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:10px;background:color-mix(in srgb,var(--customer-surface,#f8fbfc) 72%,transparent);box-shadow:inset 0 0 36px rgba(10,127,144,.06)}.token-stage__bar{position:relative;z-index:1;height:34px;box-shadow:0 0 0 1px rgba(20,33,38,.08)}.token-stage__bar i{background:var(--customer-cyan,#0a7f90)}.token-stage__bar b{display:block;height:100%;background:var(--customer-copper,#9c641f);transition:width .25s}.token-stage__flow{position:absolute;inset:18px;overflow:hidden;pointer-events:none}.token-stage__flow i{position:absolute;left:-12%;display:block;border-radius:50%;background:var(--customer-cyan,#0a7f90);box-shadow:0 0 12px color-mix(in srgb,var(--customer-cyan,#0a7f90) 45%,transparent);animation:token-flow 7s linear infinite}.token-stage__flow i:nth-child(3n){background:var(--customer-copper,#9c641f);box-shadow:0 0 12px color-mix(in srgb,var(--customer-copper,#9c641f) 42%,transparent)}@keyframes token-flow{0%{transform:translate3d(0,0,0) scale(.72);opacity:0}12%{opacity:.8}55%{transform:translate3d(62vw,-4px,0) scale(1);opacity:.64}100%{transform:translate3d(118vw,3px,0) scale(.76);opacity:0}}.cache-toggle{padding:8px 10px;border:1px solid var(--customer-line,rgba(20,33,38,.12));border-radius:6px;background:color-mix(in srgb,var(--customer-cyan-soft,rgba(10,127,144,.1)) 55%,transparent)}
@media (max-width:620px){.billing-panel{width:calc(100% + 32px);padding:16px}.billing-hero{padding:30px 18px}.billing-section{padding-top:24px}}
</style>
