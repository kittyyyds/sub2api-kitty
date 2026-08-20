import { describe, expect, it } from 'vitest'
import { injectModelPricingLinks } from '../homeContent'

const customHome = `
  <div class="kitty-home-v2">
    <header><div class="kh-actions"><a class="kh-login" href="/login">Login</a></div></header>
    <main><div class="kh-ctas"><a href="/login">Start</a></div></main>
    <footer><div class="kh-copyrow"><a href="/docs">Docs</a></div></footer>
  </div>
`

describe('injectModelPricingLinks', () => {
  it('adds public pricing links to the header, hero, and footer', () => {
    const document = new DOMParser().parseFromString(injectModelPricingLinks(customHome), 'text/html')
    const links = [...document.querySelectorAll<HTMLAnchorElement>('a[data-model-pricing-link]')]

    expect(links).toHaveLength(3)
    expect(links.map((link) => link.getAttribute('data-model-pricing-link'))).toEqual([
      'header',
      'hero',
      'footer'
    ])
    expect(links.every((link) => link.getAttribute('href') === '/model-plaza')).toBe(true)
    expect(links[0]?.textContent).toContain('模型价格')
    expect(links[0]?.textContent).toContain('Model Pricing')
    expect(document.querySelector('style[data-model-pricing-link="styles"]')?.textContent).toContain(
      '.kh-price-link'
    )
  })

  it('is idempotent when content is rendered repeatedly', () => {
    const once = injectModelPricingLinks(customHome)
    const twice = injectModelPricingLinks(once)
    const document = new DOMParser().parseFromString(twice, 'text/html')

    expect(document.querySelectorAll('a[data-model-pricing-link]')).toHaveLength(3)
  })
})
