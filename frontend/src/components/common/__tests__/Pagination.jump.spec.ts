import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import Pagination from '../Pagination.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

function mountPagination() {
  return mount(Pagination, {
    props: { total: 200, page: 1, pageSize: 20, showJump: true, showPageSizeSelector: false },
    global: { stubs: { Icon: true } }
  })
}

describe('pagination jump input', () => {
  it('jumps using the virtual page picker on mobile and desktop', async () => {
    const wrapper = mountPagination()
    await wrapper.get('.btn').trigger('click')
    const input = wrapper.get('input[type="number"]')
    await input.setValue('3')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('update:page')).toEqual([[3]])
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('rejects a jump beyond the known page range', async () => {
    const wrapper = mountPagination()
    await wrapper.get('.btn').trigger('click')
    await wrapper.get('input').setValue('99')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('update:page')).toBeUndefined()
  })

  it('ignores an empty jump input', async () => {
    const wrapper = mountPagination()
    await wrapper.get('.btn').trigger('click')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('update:page')).toBeUndefined()
  })
})
