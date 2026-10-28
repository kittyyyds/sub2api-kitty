import { describe, expect, it } from 'vitest'
import { defaultIQBootstrap } from '@/api/admin/autoConfig'
import { defaultOpenAITwoFAAdmissionConfig } from '../openaiTwoFAAdmission'
import { unifiedIQAdmission } from '../unifiedIQAdmission'
describe('legacy admission template adapter', () => {
 const old={enabled:true,platform:'openai',iq_bootstrap:{...defaultIQBootstrap(),enabled:true,quarantine_enabled:true,quarantine_group_id:44,correct_required:7,interval_seconds:90,passed_concurrency:3}}
 it('preserves legacy values and stops new enrollments in the old engine', () => {
  const result=unifiedIQAdmission(old)
  expect(result.openai_two_fa_admission).toEqual({...defaultOpenAITwoFAAdmissionConfig(),enabled:true,pass_count:7,wait_seconds:90,passed_concurrency:3,isolation_count:6,isolation_group_id:44})
  expect(result.iq_bootstrap.enabled).toBe(false)
  expect(old.iq_bootstrap.enabled).toBe(true)
  expect(unifiedIQAdmission({...old,...result})).toEqual(result)
 })
 it('never enables a formerly inactive template', () => {
  expect(unifiedIQAdmission({...old,enabled:false}).openai_two_fa_admission.enabled).toBe(false)
  expect(unifiedIQAdmission({...old,platform:'anthropic'}).openai_two_fa_admission.enabled).toBe(false)
 })
 it('retains an already enabled native policy', () => {
  const native={...defaultOpenAITwoFAAdmissionConfig(),enabled:true,isolation_group_id:5,isolation_count:9}
  expect(unifiedIQAdmission({...old,openai_two_fa_admission:native}).openai_two_fa_admission).toEqual(native)
 })
 it('does not silently enable a previously disabled isolation target', () => {
  expect(unifiedIQAdmission({...old,iq_bootstrap:{...old.iq_bootstrap,quarantine_enabled:false}}).openai_two_fa_admission.isolation_group_id).toBe(0)
 })
})
