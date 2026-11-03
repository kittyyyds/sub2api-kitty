import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PelicanTestFields from '../PelicanTestFields.vue'
import { CANDY_PROMPT } from '@/utils/intelligenceTest'
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
const SelectStub = { name: 'Select', props: ['modelValue', 'options'], emits: ['update:modelValue'], template: '<div class="select-stub" />' }
const config = { question_kind: 'pelican' as const, prompt: 'draw a pelican', reasoning_effort: 'high', parallel_count: 4 }
function render(modelValue: Record<string, unknown> = config) {
  return mount(PelicanTestFields, { props: { modelValue: modelValue as any }, global: { stubs: { Select: SelectStub, TextArea: true, Input: true } } })
}
describe('PelicanTestFields question kinds', () => {
  it('offers only answer-based question kinds', () => {
    const wrapper = render()
    expect(wrapper.findAllComponents(SelectStub)[0].props('options').map((o: { value: string }) => o.value)).toEqual(['candy', 'pelican'])
    wrapper.unmount()
  })
  it('updates the prompt when switching to candy', () => {
    const wrapper = render()
    wrapper.findAllComponents(SelectStub)[0].vm.$emit('update:modelValue', 'candy')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([{ ...config, question_kind: 'candy', prompt: CANDY_PROMPT }])
    wrapper.unmount()
  })
})
