import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { resolveKittySite } from '../siteIdentity'

afterEach(() => vi.unstubAllEnvs())

const commonHashes = {
  '/images/heyroute/support/qq-qr.png': 'b039a2791eb6d9f950977bfcd3876c37a801c3bd39b1fb885b5a4c8af715aa3a',
  '/images/heyroute/support/wechat-qr.png': '245cff833b0eeebe44fd58994b6b102b5c3531bd3041a15a4303e2dbdd08b6c4',
  '/images/heyroute/community/telegram-qr.png': '657e4dd72fb498d4d2bccc1a5bf07cce42e8f022c28b01cf78cb433803048acd',
}

describe('site identity and original QR assets', () => {
  it('keeps Custom as the default and rejects misspelled deployment selectors', () => {
    expect(resolveKittySite().id).toBe('custom')
    expect(resolveKittySite('').id).toBe('custom')
    expect(() => resolveKittySite('kitty-api')).toThrow('Invalid VITE_KITTY_SITE')
  })

  it('keeps Custom login and API links on its own origin', () => {
    expect(resolveKittySite('custom', 'https://custom.kittyapi.xyz').primaryUrl).toBe('https://custom.kittyapi.xyz')
    expect(resolveKittySite('custom', 'https://preview.example').domesticUrl).toBe('https://preview.example')
  })

  it.each([
    {
      id: 'kittyapi',
      prefix: '/sites/kittyapi',
      group: 'https://qm.qq.com/q/oRR8GUdEPK',
      groupHash: '0ea2fcfe962c14f200caf2d15916ba040f1052a796a191cdc7f2fe4bf4d58c43',
      legacyCommunityHash: 'fda23a84984aaf5add967f26fffb9f5e9c6e506c9afb43e2edd41896655bc47c',
    },
    {
      id: 'kittyyyds',
      prefix: '',
      group: 'https://qm.qq.com/q/Wr3pNr9zMe',
      groupHash: '495bba2e9160958184968442a0454d0b901cc9d9aff7a57abd56a2232b9a9dbd',
      legacyCommunityHash: '495bba2e9160958184968442a0454d0b901cc9d9aff7a57abd56a2232b9a9dbd',
    },
  ])('selects $id contacts and preserves all original image bytes', async (site) => {
    vi.resetModules()
    vi.stubEnv('VITE_KITTY_SITE', site.id)
    const { kittySite } = await import('../site')
    const { customerSupportContacts, afterSalesGroupContact, telegramGroupContact } = await import('../customerContact')

    expect(kittySite.primaryUrl).toBe(`https://${site.id}.xyz`)
    expect(kittySite.domesticUrl).toBe(`https://cdn.${site.id}.xyz`)
    expect(afterSalesGroupContact).toEqual({ href: site.group, qrSrc: `${site.prefix}/qq-group-qr.png` })
    expect(customerSupportContacts.qq.qrSrc).toBe(`${site.prefix}/images/heyroute/support/qq-qr.png`)
    expect(customerSupportContacts.wechat.qrSrc).toBe(`${site.prefix}/images/heyroute/support/wechat-qr.png`)
    expect(telegramGroupContact.qrSrc).toBe(`${site.prefix}/images/heyroute/community/telegram-qr.png`)

    const hashes = {
      ...commonHashes,
      '/qq-group-qr.png': site.groupHash,
      '/images/heyroute/community/qq-qr.png': site.legacyCommunityHash,
    }
    for (const [asset, hash] of Object.entries(hashes)) {
      const file = resolve(process.cwd(), 'public', `.${site.prefix}${asset}`)
      expect(createHash('sha256').update(readFileSync(file)).digest('hex'), file).toBe(hash)
    }
  })
})
