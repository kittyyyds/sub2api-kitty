<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { afterSalesGroupContact, customerSupportContacts } from '@/config/customerContact'

type ContactTab = 'service' | 'after-sales' | 'business'
const props = withDefaults(defineProps<{ show: boolean; initialTab?: ContactTab }>(), { initialTab: 'service' })
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const activeTab = ref<ContactTab>('service')
const tabs = computed(() => [
  { id: 'service' as const, label: t('nav.contactServiceTab') },
  { id: 'after-sales' as const, label: t('nav.contactAfterSalesTab') },
  { id: 'business' as const, label: t('nav.contactBusinessTab') },
])
const tabElements = ref<HTMLButtonElement[]>([])
watch(() => [props.show, props.initialTab], () => {
  if (props.show) activeTab.value = props.initialTab
}, { immediate: true })

async function moveTab(event: KeyboardEvent, index: number) {
  let next = index
  if (event.key === 'ArrowRight') next = (index + 1) % tabs.value.length
  else if (event.key === 'ArrowLeft') next = (index + tabs.value.length - 1) % tabs.value.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = tabs.value.length - 1
  else return
  event.preventDefault()
  activeTab.value = tabs.value[next]!.id
  await nextTick()
  tabElements.value[next]?.focus()
}
</script>

<template>
  <BaseDialog :show="show" :title="t('nav.contactUs')" width="wide" close-on-click-outside @close="emit('close')">
    <div class="contact-dialog-tabs" role="tablist" :aria-label="t('nav.contactCategories')">
      <button v-for="(tab, index) in tabs" :id="`kitty-contact-tab-${tab.id}`" :key="tab.id" ref="tabElements" type="button" role="tab"
        :aria-selected="activeTab === tab.id" :aria-controls="`kitty-contact-panel-${tab.id}`" :tabindex="activeTab === tab.id ? 0 : -1"
        @click="activeTab = tab.id" @keydown="moveTab($event, index)">{{ tab.label }}</button>
    </div>
    <div :id="`kitty-contact-panel-${activeTab}`" role="tabpanel" :aria-labelledby="`kitty-contact-tab-${activeTab}`" class="contact-dialog-content">
      <div class="contact-dialog-grid">
        <article v-if="activeTab !== 'after-sales'" class="contact-dialog-card">
          <h2>{{ t('nav.customerSupportWechat') }}</h2>
          <div class="contact-qr-well"><img :src="customerSupportContacts.wechat.qrSrc" :alt="t('nav.customerSupportWechatQrAlt')" width="220" height="220"></div>
          <p class="contact-caption">{{ t('nav.contactScan') }}</p>
        </article>
        <article v-if="activeTab !== 'business'" class="contact-dialog-card">
          <h2>{{ t('nav.contactQqGroup') }}</h2>
          <div class="contact-qr-well"><img :src="afterSalesGroupContact.qrSrc" :alt="t('nav.afterSalesGroupQrHint')" width="220" height="220"></div>
          <a :href="afterSalesGroupContact.href" target="_blank" rel="noopener noreferrer" class="contact-caption">{{ t('nav.contactJoinGroup') }}</a>
        </article>
        <article v-if="activeTab === 'business'" class="contact-dialog-card">
          <h2>{{ t('nav.customerSupportQq') }}</h2>
          <div class="contact-qr-well"><img :src="customerSupportContacts.qq.qrSrc" :alt="t('nav.customerSupportQqQrAlt')" width="220" height="220"></div>
          <a :href="customerSupportContacts.qq.href" target="_blank" rel="noopener noreferrer" class="contact-caption">{{ t('nav.contactOpenQq') }}</a>
        </article>
        <article v-if="activeTab === 'after-sales'" class="contact-dialog-card contact-dialog-ticket">
          <h2>{{ t('nav.tickets') }}</h2>
          <div class="contact-ticket-well"><span aria-hidden="true">?</span><p>{{ t('nav.contactTicketHint') }}</p></div>
          <router-link to="/tickets" class="contact-caption" @click="emit('close')">{{ t('nav.contactOpenTicket') }}</router-link>
        </article>
      </div>
    </div>
    <footer class="contact-dialog-footer">
      <span>{{ t('nav.customerSupportQq') }}</span>
      <a :href="customerSupportContacts.qq.href" target="_blank" rel="noopener noreferrer">{{ customerSupportContacts.qq.account }}</a>
      <small v-if="activeTab === 'business'">{{ t('nav.contactBusinessHint') }}</small>
    </footer>
  </BaseDialog>
</template>

<style scoped>
:global(.modal-content:has(.contact-dialog-tabs)) { max-width: 896px; border-radius: 16px; }
:global(.modal-content:has(.contact-dialog-tabs) .modal-header) { padding: 16px 24px; border-bottom: 1px solid var(--kc-line, #e5e7eb); }
:global(.modal-content:has(.contact-dialog-tabs) .modal-body) { padding: 16px 24px; }
.contact-dialog-tabs { display: flex; max-width: 512px; gap: 4px; padding: 6px; margin: 0 auto 20px; border-radius: 18px; background: var(--kc-panel-muted, #f5f5f4); }
.contact-dialog-tabs button { flex: 1; border-radius: 13px; padding: 11px 8px; font-size: 14px; font-weight: 600; color: var(--kc-muted); }
.contact-dialog-tabs button[aria-selected='true'] { color: var(--kc-accent-strong); background: var(--kc-panel); box-shadow: 0 5px 20px rgb(23 27 26 / 5%); }
.contact-dialog-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 20px; }
.contact-dialog-card { min-width: 0; display: flex; flex-direction: column; align-items: center; gap: 16px; padding: 18px 20px; border-radius: 18px; background: var(--kc-panel); box-shadow: 0 16px 36px rgb(23 27 26 / 5%); text-align: center; }
.contact-dialog-card h2 { font-size: 20px; font-weight: 700; color: var(--kc-ink); }
.contact-qr-well,.contact-ticket-well { display: flex; width: 100%; min-height: 252px; align-items: center; justify-content: center; border-radius: 16px; padding: 16px; background: var(--kc-panel-muted, #f8f7f4); }
.contact-dialog-card img { width: min(100%,220px); aspect-ratio: 1; height: auto; object-fit: contain; border-radius: 8px; background: white; }
.contact-caption { font-size: 14px; line-height: 1.7; color: var(--kc-accent-strong); font-weight: 600; }
a.contact-caption:hover { text-decoration: underline; text-underline-offset: 4px; }
.contact-ticket-well { flex-direction: column; gap: 16px; color: var(--kc-muted); font-size: 14px; line-height: 1.8; }
.contact-ticket-well > span { display: grid; width: 64px; height: 64px; place-content: center; border: 2px solid var(--kc-accent); border-radius: 50%; color: var(--kc-accent-strong); font-size: 40px; font-weight: 600; }
.contact-dialog-footer { display: flex; flex-direction: column; align-items: center; gap: 10px; margin-top: 22px; }
.contact-dialog-footer > span { color: var(--kc-accent-strong); font-size: 14px; font-weight: 600; }
.contact-dialog-footer a { font-size: 26px; font-weight: 700; color: var(--kc-ink); }
.contact-dialog-footer small { color: var(--kc-muted); font-size: 13px; }
@media(max-width: 600px) {
  :global(.modal-content:has(.contact-dialog-tabs) .modal-body) { padding: 16px; }
  .contact-dialog-grid { gap: 10px; }
  .contact-dialog-card { padding: 16px 10px; gap: 12px; }
  .contact-dialog-card h2 { font-size: 16px; }
  .contact-qr-well,.contact-ticket-well { min-height: 0; padding: 8px; flex: 1; }
  .contact-dialog-footer { margin-top: 20px; }
}
</style>
