import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import IQBootstrapSettings from '../IQBootstrapSettings.vue'
import { defaultOpenAITwoFAAdmissionConfig, openaiTwoFAAdmissionError } from '@/utils/openaiTwoFAAdmission'
import type { Group } from '@/types'
const groups = [{id:1,name:'生产',platform:'openai',status:'active'}, {id:2,name:'隔离',platform:'openai',status:'active'}, {id:3,name:'停用',platform:'openai',status:'inactive'}, {id:4,name:'其他',platform:'anthropic',status:'active'}] as Group[]
describe('单入口新版糖果准入', () => {
 it('shows all groups, requires an active OpenAI target and has no optional quarantine switch', async () => {
  const value = {...defaultOpenAITwoFAAdmissionConfig(),enabled:true}
  const w=mount(IQBootstrapSettings,{props:{modelValue:value,groups}})
  expect(w.findAll('option').filter(o=>o.text().includes(' · #'))).toHaveLength(4)
  expect(w.find('[data-testid="quarantine-enabled"]').exists()).toBe(false)
  expect(openaiTwoFAAdmissionError(value)).not.toBeNull()
  expect(w.get('[data-testid="admission-group"] option[value="3"]').attributes('disabled')).toBeDefined()
  expect(w.get('[data-testid="admission-group"] option[value="4"]').attributes('disabled')).toBeDefined()
  await w.get('[data-testid="admission-group"]').setValue('2')
  const selected=w.emitted('update:modelValue')!.at(-1)![0] as typeof value
  expect(selected.isolation_group_id).toBe(2)
  expect(openaiTwoFAAdmissionError(selected)).toBeNull()
  expect(w.text()).toContain('独立于上方首次配置开关和平台选择')
 })
 it('supports configurable 29 streaks and the native 1-concurrency lower bound', async () => {
  const value={...defaultOpenAITwoFAAdmissionConfig(),enabled:true,isolation_group_id:2}
  const w=mount(IQBootstrapSettings,{props:{modelValue:value,groups}})
  await w.get('[data-testid="admission-isolation-count"]').setValue(9)
  expect(w.emitted('update:modelValue')!.at(-1)![0]).toMatchObject({isolation_count:9})
  expect(w.get('[data-testid="admission-passed_concurrency"]').attributes('min')).toBe('1')
  expect(w.get('[data-testid="admission-effort"] option[value="ultra"]').exists()).toBe(true)
 })
 it('retains a missing group ID visibly rather than silently picking another', () => {
  const w=mount(IQBootstrapSettings,{props:{modelValue:{...defaultOpenAITwoFAAdmissionConfig(),enabled:true,isolation_group_id:99},groups}})
  expect(w.get('[data-testid="admission-group"] option[value="99"]').attributes('disabled')).toBeDefined()
 })
})
