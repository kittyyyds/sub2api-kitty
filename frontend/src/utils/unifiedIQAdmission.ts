import { defaultIQBootstrap, type AutoConfig } from '@/api/admin/autoConfig'
import { defaultOpenAITwoFAAdmissionConfig } from '@/utils/openaiTwoFAAdmission'

// Only migrate the editable template, never account snapshots or test progress.
export function unifiedIQAdmission(config: Pick<AutoConfig, 'enabled' | 'platform' | 'iq_bootstrap' | 'openai_two_fa_admission'>) {
  const legacy = { ...defaultIQBootstrap(), ...config.iq_bootstrap }
  let admission = { ...defaultOpenAITwoFAAdmissionConfig(), ...config.openai_two_fa_admission }
  if (legacy.enabled && !admission.enabled) {
    admission = {
      enabled: config.enabled && config.platform === 'openai',
      model_id: legacy.model,
      reasoning_effort: legacy.reasoning,
      wait_seconds: legacy.interval_seconds,
      pass_count: legacy.correct_required,
      passed_concurrency: legacy.passed_concurrency,
      isolation_count: 6,
      isolation_group_id: legacy.quarantine_enabled ? legacy.quarantine_group_id : 0,
    }
  }
  return {
    iq_bootstrap: { ...legacy, enabled: false },
    openai_two_fa_admission: admission,
  }
}
