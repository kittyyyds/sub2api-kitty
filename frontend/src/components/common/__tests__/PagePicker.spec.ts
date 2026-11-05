import { mount } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import PagePicker from '../PagePicker.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => `${k}${p ? JSON.stringify(p) : ''}` }) }))

describe('PagePicker', () => {
  it('virtualizes page numbers instead of rendering the full set', async () => {
    const w = mount(PagePicker, { props: { page: 1, pages: 100000 } })
    await w.get('button').trigger('click')
    expect(w.findAll('[data-testid="page-picker-scroll"] button').length).toBeLessThanOrEqual(13)
    const scroll = w.get('[data-testid="page-picker-scroll"]')
    scroll.element.scrollTop = 36000
    await scroll.trigger('scroll')
    expect(w.text()).toContain('1001')
    w.unmount()
  })
  it('supports exact numeric jumps and rejects fractions/out-of-range input', async () => {
    const w = mount(PagePicker, { props: { page: 2, pages: 40 } })
    await w.get('button').trigger('click')
    for (const input of ['', '-1', '0', '2.5', '41']) {
      await w.get('input').setValue(input); await w.get('form').trigger('submit')
      expect(w.emitted('select')).toBeUndefined()
    }
    await w.get('input').setValue('17'); await w.get('form').trigger('submit')
    expect(w.emitted('select')).toEqual([[17]])
    expect(w.find('form').exists()).toBe(false)
    w.unmount()
  })
  it('labels estimated totals and closes with Escape', async () => {
    const w = mount(PagePicker, { props: { page: 2, pages: 3, exact: false } })
    expect(w.text()).toContain('optimizations.pagesAtLeast')
    await w.get('button').trigger('click')
    expect(w.text()).toContain('optimizations.lowerBoundHint')
    await w.get('input').trigger('keydown', { key: 'Escape' })
    expect(w.find('form').exists()).toBe(false)
    w.unmount()
  })
})
