import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import IQBootstrapSettings from '../IQBootstrapSettings.vue'
import { defaultIQBootstrap } from '@/api/admin/autoConfig'
import type { Group } from '@/types'

const groups = [
 {id:1,name:'生产',platform:'openai',status:'active'},
 {id:2,name:'已有隔离',platform:'openai',status:'active'},
 {id:3,name:'停用组',platform:'openai',status:'inactive'},
 {id:4,name:'其他平台',platform:'anthropic',status:'active'}
] as Group[]
describe('隔离分组选择',()=>{
 it('展示全部已有分组，不默认选择，不选组时仍可首次验证',async()=>{
  const value={...defaultIQBootstrap(),enabled:true}
  const wrapper=mount(IQBootstrapSettings,{props:{modelValue:value,groups,platform:'openai'}})
  expect(wrapper.findAll('option').filter(o=>o.text().includes(' · #'))).toHaveLength(4)
  expect((wrapper.get('[data-testid="quarantine-enabled"]').element as HTMLInputElement).disabled).toBe(true)
  expect(wrapper.get('[data-testid="quarantine-group"]').find('option[value="3"]').attributes('disabled')).toBeDefined()
  await wrapper.get('[data-testid="quarantine-group"]').setValue('2')
  const selected=wrapper.emitted('update:modelValue')!.at(-1)![0] as typeof value
  expect(selected.quarantine_group_id).toBe(2)
  expect(selected.quarantine_enabled).toBe(false)
  await wrapper.setProps({modelValue:{...selected,quarantine_enabled:true}})
  expect((wrapper.get('[data-testid="quarantine-enabled"]').element as HTMLInputElement).disabled).toBe(false)
  await wrapper.get('[data-testid="quarantine-group"]').setValue('0')
  const cleared=wrapper.emitted('update:modelValue')!.at(-1)![0] as typeof value
  expect(cleared.enabled).toBe(true)
  expect(cleared.quarantine_enabled).toBe(false)
 })
})
