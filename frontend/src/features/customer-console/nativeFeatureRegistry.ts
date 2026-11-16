import { defineAsyncComponent, type Component } from 'vue'

export const NATIVE_CUSTOM_FEATURES = [
  {
    id: 'billing-rules',
    path: '/custom/billing-rules',
    labelKey: 'nav.billingRules',
    fallbackLabel: '计费规则',
    descriptionKey: 'native.featureDescription.billing',
    headerDescription: 'Token、缓存与倍率估算',
    icon: 'calculator'
  },
  {
    id: 'docs',
    path: '/custom/docs',
    labelKey: 'nav.gettingStarted',
    fallbackLabel: '新手必看',
    descriptionKey: 'native.featureDescription.docs',
    headerDescription: '使用说明与接入文档',
    icon: 'book'
  }
] as const

export type NativeCustomFeatureId = (typeof NATIVE_CUSTOM_FEATURES)[number]['id']

const NATIVE_CUSTOM_FEATURE_IDS = new Set<string>(
  NATIVE_CUSTOM_FEATURES.map(({ id }) => id)
)
// Keep retired IDs reserved so saved legacy custom menus cannot restore them.
const LEGACY_CUSTOM_FEATURE_IDS = new Set(['image-generator', 'reset-radar'])

const nativeFeatureComponents: Record<NativeCustomFeatureId, Component> = {
  'billing-rules': defineAsyncComponent(
    () => import('@/views/user/native/BillingRulesPanel.vue')
  ),
  docs: defineAsyncComponent(
    () => import('@/views/user/native/FeishuGuidePanel.vue')
  )
}

export function isNativeCustomFeatureId(value: unknown): value is NativeCustomFeatureId {
  return typeof value === 'string' && NATIVE_CUSTOM_FEATURE_IDS.has(value)
}

export function isReservedCustomerFeatureId(value: unknown): boolean {
  return isNativeCustomFeatureId(value)
    || (typeof value === 'string' && LEGACY_CUSTOM_FEATURE_IDS.has(value))
}

export function getNativeFeatureComponent(value: unknown): Component | null {
  return isNativeCustomFeatureId(value) ? nativeFeatureComponents[value] : null
}

export function getNativeFeature(value: unknown) {
  return isNativeCustomFeatureId(value)
    ? NATIVE_CUSTOM_FEATURES.find(({ id }) => id === value) ?? null
    : null
}
