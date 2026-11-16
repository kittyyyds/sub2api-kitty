const MODEL_PRICING_LINK_ATTR = 'data-model-pricing-link'

function createBilingualLink(
  document: Document,
  className: string,
  chinese: string,
  english: string,
  placement: string
): HTMLAnchorElement {
  const link = document.createElement('a')
  link.className = className
  link.href = '/model-plaza'
  link.setAttribute(MODEL_PRICING_LINK_ATTR, placement)

  const zh = document.createElement('span')
  zh.className = 'zh'
  zh.textContent = chinese
  const en = document.createElement('span')
  en.className = 'en'
  en.textContent = english
  link.append(zh, en)
  return link
}

/** Adds public model-pricing entry points to the current Kitty custom home page. */
export function injectModelPricingLinks(homeContent: string): string {
  if (!homeContent.trim() || typeof DOMParser === 'undefined') return homeContent

  const document = new DOMParser().parseFromString(homeContent, 'text/html')
  if (!document.body.querySelector(`[${MODEL_PRICING_LINK_ATTR}]`)) {
    const responsiveStyle = document.createElement('style')
    responsiveStyle.setAttribute(MODEL_PRICING_LINK_ATTR, 'styles')
    responsiveStyle.textContent = '@media(max-width:640px){.kh-price-link{display:none!important}}'
    document.body.prepend(responsiveStyle)

    const headerActions = document.body.querySelector('.kh-actions')
    const loginLink = headerActions?.querySelector('.kh-login')
    if (headerActions) {
      const link = createBilingualLink(document, 'kh-login kh-price-link', '模型价格', 'Model Pricing', 'header')
      headerActions.insertBefore(link, loginLink ?? null)
    }

    const heroActions = document.body.querySelector('.kh-ctas')
    if (heroActions) {
      const link = createBilingualLink(document, 'kh-secondary', '查看模型价格', 'View Model Pricing', 'hero')
      heroActions.appendChild(link)
    }

    const footerLinks = document.body.querySelector('.kh-copyrow')
    if (footerLinks) {
      const link = createBilingualLink(document, '', '模型价格', 'Model Pricing', 'footer')
      footerLinks.appendChild(link)
    }
  }

  return document.body.innerHTML
}
