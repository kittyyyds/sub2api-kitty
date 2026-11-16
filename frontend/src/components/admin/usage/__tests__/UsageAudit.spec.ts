import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { AdminUsageLog } from '@/types'

import UsageAuditTable from '../UsageAuditTable.vue'
import UsageAuditDetailDialog from '../UsageAuditDetailDialog.vue'

const messages: Record<string, string> = {
  'admin.usage.audit.status.anomaly': 'Anomaly',
  'admin.usage.audit.localEstimateDisabled': 'Local cross-check disabled',
  'admin.usage.audit.reasonsMap.local_estimate_disabled': 'Local cross-check disabled; upstream usage and normal billing are retained',
  'admin.usage.audit.status.missing': 'Missing',
  'admin.usage.audit.status.warning': 'Review',
  'admin.usage.audit.status.pass': 'Trusted',
  'admin.usage.audit.status.unverifiable': 'Unverifiable',
  'admin.usage.audit.status.not_applicable': 'N/A',
  'admin.usage.audit.confidence.high': 'High',
  'admin.usage.audit.evidence.upstream_raw': 'Upstream raw',
  'admin.usage.audit.evidence.gateway_response': 'Gateway response',
  'admin.usage.audit.noRawUsage': 'Raw usage was not captured',
  'admin.usage.audit.columns.status': 'Result',
  'admin.usage.audit.columns.request': 'Request',
  'admin.usage.audit.columns.modelAccount': 'Model / account',
  'admin.usage.audit.columns.upstream': 'Reported usage',
  'admin.usage.audit.columns.local': 'Local estimate',
  'admin.usage.audit.columns.deviation': 'Delta / deviation',
  'admin.usage.audit.columns.cache': 'Cache',
  'admin.usage.audit.columns.evidence': 'Evidence',
  'admin.usage.audit.columns.action': 'Detail',
  'admin.usage.audit.inputTokens': 'Input {value}',
  'admin.usage.audit.outputTokens': 'Output {value}',
  'admin.usage.audit.cacheTokens': 'Cache {value}',
  'admin.usage.audit.hitRate': 'Hit {value}',
  'admin.usage.audit.inspect': 'Inspect audit evidence',
  'admin.usage.audit.empty': 'No audit records',
  'admin.usage.audit.detailTitle': 'Usage Authenticity Audit',
  'admin.usage.audit.requestInfo': 'Request information',
  'admin.usage.audit.comparison': 'Token comparison',
  'admin.usage.audit.metric': 'Metric',
  'admin.usage.audit.reported': 'Reported',
  'admin.usage.audit.estimated': 'Estimated',
  'admin.usage.audit.delta': 'Delta',
  'admin.usage.audit.deviation': 'Deviation',
  'admin.usage.audit.input': 'Input tokens',
  'admin.usage.audit.output': 'Output tokens',
  'admin.usage.audit.cacheCreation': 'Cache write',
  'admin.usage.audit.cacheRead': 'Cache read',
  'admin.usage.audit.evidenceInfo': 'Evidence',
  'admin.usage.audit.confidenceLabel': 'Confidence',
  'admin.usage.audit.sourceLabel': 'Source',
  'admin.usage.audit.protocolLabel': 'Protocol',
  'admin.usage.audit.estimatorLabel': 'Estimator',
  'admin.usage.audit.truncated': 'Response snapshot was truncated',
  'admin.usage.audit.reasons': 'Reasons',
  'admin.usage.audit.rawUsage': 'Raw usage',
  'usage.model': 'Model',
  'usage.requestedModel': 'Requested model',
  'usage.sentUpstreamModel': 'Sent upstream model',
  'usage.upstreamResponseModel': 'Upstream response model',
  'usage.type': 'Type',
  'admin.usage.account': 'Account',
  'admin.usage.group': 'Group',
}

const translate = (key: string, params?: Record<string, unknown>) => {
  let value = messages[key] ?? key
  for (const [name, replacement] of Object.entries(params ?? {})) {
    value = value.replace(`{${name}}`, String(replacement))
  }
  return value
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate,
      te: (key: string) => key in messages,
    }),
  }
})

const IconStub = defineComponent({
  template: '<span />',
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean, title: { type: String, required: true } },
  template: '<div v-if="show"><h2>{{ title }}</h2><slot /></div>',
})

const audit = {
  version: 1,
  status: 'anomaly' as const,
  confidence: 'high' as const,
  evidence_source: 'upstream_raw' as const,
  protocol: 'openai_responses',
  estimator: 'responses_tiktoken_v1',
  upstream: {
    input_tokens: 120,
    total_input_tokens: 120,
    output_tokens: 80,
    cache_creation_tokens: 4,
    cache_read_tokens: 20,
  },
  local: {
    input_tokens: 100,
    total_input_tokens: 100,
    output_tokens: 70,
  },
  delta: { input_tokens: 20, output_tokens: 10 },
  deviation_pct: { input_tokens: 20, output_tokens: 14.29 },
  cache_hit_rate: 16.67,
  reasons: ['input_token_deviation_high'],
  upstream_raw_usage: { input_tokens: 120, output_tokens: 80 },
  capture_truncated: false,
}

const row = {
  id: 7,
  user_id: 11,
  api_key_id: 3,
  account_id: 4,
  request_id: 'req-audit-7',
  model: 'gpt-5.6',
  upstream_model: 'gpt-5.6-20260101',
  upstream_response_model: 'gpt-5.6-20260101',
  group_id: 2,
  created_at: '2026-08-20T10:00:00Z',
  user: { email: 'audit@example.com' },
  account: { id: 4, name: 'Primary account' },
  group: { id: 2, name: 'OpenAI' },
  request_type: 'sync',
  usage_audit: audit,
} as unknown as AdminUsageLog

describe('image output evidence', () => {
  it('shows measured dimensions separately from upstream declarations', () => {
    const imageRow = { ...row, image_count: 1, usage_audit: { ...audit, image_output_evidence: {
      version: 1, requested_model: 'public-image', upstream_model: 'actual-image', reported_model: 'actual-image',
      model_status: 'consistent', requested_size: '1024x1024', size_status: 'different', size_evidence: 'image_header',
      outputs: [{ declared_size: '1024x1024', measured_size: '64x32' }], postprocessed: false,
    } } } as AdminUsageLog
    const wrapper = mount(UsageAuditDetailDialog, { props: { show: true, row: imageRow }, global: { stubs: { BaseDialog: BaseDialogStub, Icon: IconStub } } })
    const section = wrapper.get('[data-testid="image-output-evidence"]')
    expect(section.text()).toContain('1024x1024')
    expect(section.text()).toContain('64x32')
    expect(section.text()).toContain('admin.usage.imageEvidence.sizeStatus.different')
  })
  it('does not manufacture evidence for older image records', () => {
    const wrapper = mount(UsageAuditDetailDialog, { props: { show: true, row: { ...row, image_count: 1 } as AdminUsageLog }, global: { stubs: { BaseDialog: BaseDialogStub, Icon: IconStub } } })
    expect(wrapper.get('[data-testid="image-output-evidence"]').text()).toContain('admin.usage.imageEvidence.noEvidence')
  })
})

describe('UsageAuditTable', () => {
  it('renders the audit comparison and emits inspect/user actions', async () => {
    const wrapper = mount(UsageAuditTable, {
      props: { data: [row] },
      global: { stubs: { Icon: IconStub } },
    })

    expect(wrapper.text()).toContain('Anomaly')
    expect(wrapper.text()).toContain('Input 120')
    expect(wrapper.text()).toContain('Input 100')
    expect(wrapper.text()).toContain('+20 / +20.0%')
    expect(wrapper.text()).toContain('Hit 16.7%')

    await wrapper.get('[data-testid="audit-inspect"]').trigger('click')
    await wrapper.get('[data-testid="audit-user-link"]').trigger('click')

    expect(wrapper.emitted('inspect')?.[0]).toEqual([row])
    expect(wrapper.emitted('userClick')?.[0]).toEqual([11, 'audit@example.com'])
  })

  it('marks a row without persisted evidence as missing', () => {
    const wrapper = mount(UsageAuditTable, {
      props: { data: [{ ...row, usage_audit: null }] },
      global: { stubs: { Icon: IconStub } },
    })

    expect(wrapper.text()).toContain('Missing')
  })
})

describe('UsageAuditDetailDialog', () => {
  it('shows the numeric comparison, evidence metadata, and raw usage only', async () => {
    const wrapper = mount(UsageAuditDetailDialog, {
      props: { show: true, row },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: IconStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Usage Authenticity Audit')
    expect(wrapper.text()).toContain('Token comparison')
    expect(wrapper.text()).toContain('Upstream raw')
    expect(wrapper.text()).toContain('responses_tiktoken_v1')
    expect(wrapper.text()).toContain('input_token_deviation_high')
    expect(wrapper.text()).toContain('input_tokens')
    expect(wrapper.text()).not.toContain('prompt')
  })
})


describe('disabled local token cross-check', () => {
  const disabledRow = {
    ...row,
    usage_audit: {
      ...audit,
      version: 2,
      status: 'unverifiable',
      confidence: 'low',
      evidence_source: 'normalized_only',
      estimator: undefined,
      local: {},
      delta: {},
      deviation_pct: {},
      reasons: ['local_estimate_disabled'],
      upstream_raw_usage: undefined,
    },
  } as AdminUsageLog

  it('labels the table row as disabled while retaining upstream usage', () => {
    const wrapper = mount(UsageAuditTable, {
      props: { data: [disabledRow] },
      global: { stubs: { Icon: IconStub } },
    })
    expect(wrapper.text()).toContain('Local cross-check disabled')
    expect(wrapper.text()).toContain('Input 120')
    expect(wrapper.text()).toContain('Cache 20')
    expect(wrapper.text()).not.toContain('Trusted')
    expect(wrapper.text()).not.toContain('Anomaly')
  })

  it('explains the disabled cross-check in the detail without fabricated estimates', async () => {
    const wrapper = mount(UsageAuditDetailDialog, {
      props: { show: true, row: disabledRow },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: IconStub } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('upstream usage and normal billing are retained')
    expect(wrapper.text()).not.toContain('responses_tiktoken_v1')
    expect(wrapper.text()).not.toContain('Trusted')
    expect(wrapper.text()).not.toContain('input_token_deviation_high')
  })
})
