import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { heyRouteCopyByLanguage } from '../heyRouteCopy'

afterEach(() => vi.unstubAllEnvs())

describe('HeyRouteCommunityDialog site isolation', () => {
  it.each([
    ['kittyapi', 'https://qm.qq.com/q/oRR8GUdEPK', '/sites/kittyapi/qq-group-qr.png'],
    ['kittyyyds', 'https://qm.qq.com/q/Wr3pNr9zMe', '/qq-group-qr.png'],
  ])('uses the same %s group as customer support', async (site, href, qrSrc) => {
    vi.resetModules()
    vi.stubEnv('VITE_KITTY_SITE', site)
    const { default: CommunityDialog } = await import('../HeyRouteCommunityDialog.vue')
    const { afterSalesGroupContact } = await import('@/config/customerContact')
    const wrapper = shallowMount(CommunityDialog, {
      props: { open: true, copy: heyRouteCopyByLanguage.zh.footer },
    })
    const platforms = wrapper.getComponent({ name: 'HeyRouteContactDialog' }).props('platforms')
    const qq = platforms.find((platform: { id: string }) => platform.id === 'qq')
    expect(qq).toMatchObject({ value: href, externalUrl: href, qrSrc })
    expect(qq.externalUrl).toBe(afterSalesGroupContact.href)
    expect(qq.qrSrc).toBe(afterSalesGroupContact.qrSrc)
    wrapper.unmount()
  })
})
