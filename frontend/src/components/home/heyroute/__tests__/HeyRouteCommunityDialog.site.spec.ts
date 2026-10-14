import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { heyRouteCopyByLanguage } from '../heyRouteCopy'

afterEach(() => vi.unstubAllEnvs())

describe('HeyRouteCommunityDialog site isolation', () => {
  it.each([
    ['kittyapi', 'zh'],
    ['kittyapi', 'en'],
    ['kittyyyds', 'zh'],
    ['kittyyyds', 'en'],
  ] as const)('preserves the original %s homepage contact in %s', async (site, language) => {
    vi.resetModules()
    vi.stubEnv('VITE_KITTY_SITE', site)
    const { default: CommunityDialog } = await import('../HeyRouteCommunityDialog.vue')
    const { afterSalesGroupContact } = await import('@/config/customerContact')
    const copy = heyRouteCopyByLanguage[language].footer
    const wrapper = shallowMount(CommunityDialog, { props: { open: true, copy } })
    const platforms = wrapper.getComponent({ name: 'HeyRouteContactDialog' }).props('platforms')
    const qq = platforms.find((platform: { id: string }) => platform.id === 'qq')
    if (site === 'kittyapi') {
      expect(qq).toMatchObject({
        value: '494038',
        valueLabel: copy.communityDialogGroupNumber,
        copyLabel: copy.communityDialogCopy,
        externalUrl: 'https://qm.qq.com/q/aP8eapBPpu',
        qrSrc: '/sites/kittyapi/images/heyroute/community/qq-qr.png',
      })
      expect(afterSalesGroupContact).toEqual({
        href: 'https://qm.qq.com/q/oRR8GUdEPK',
        qrSrc: '/sites/kittyapi/qq-group-qr.png',
      })
      expect(qq.externalUrl).not.toBe(afterSalesGroupContact.href)
      expect(qq.qrSrc).not.toBe(afterSalesGroupContact.qrSrc)
    } else {
      expect(qq).toMatchObject({
        value: 'https://qm.qq.com/q/Wr3pNr9zMe',
        valueLabel: copy.communityDialogGroupLink,
        copyLabel: copy.communityDialogCopyLink,
        externalUrl: 'https://qm.qq.com/q/Wr3pNr9zMe',
        qrSrc: '/qq-group-qr.png',
      })
      expect(qq.externalUrl).toBe(afterSalesGroupContact.href)
      expect(qq.qrSrc).toBe(afterSalesGroupContact.qrSrc)
    }
    const telegram = platforms.find((platform: { id: string }) => platform.id === 'telegram')
    expect(telegram.qrSrc).toBe(`${site === 'kittyapi' ? '/sites/kittyapi' : ''}/images/heyroute/community/telegram-qr.png`)
    wrapper.unmount()
  })
})
