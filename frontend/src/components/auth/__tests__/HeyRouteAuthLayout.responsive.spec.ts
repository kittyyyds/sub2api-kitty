import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const componentSource = readFileSync(
  resolve(__dirname, '../HeyRouteAuthLayout.vue'),
  'utf8',
)
const backdropSource = readFileSync(resolve(__dirname, '../HeyRouteAuthBackdrop.vue'), 'utf8')
const fluidSource = readFileSync(resolve(__dirname, '../../../composables/useHeyRouteFluidCanvas.ts'), 'utf8')

function mediaBlock(maxWidth: number): string {
  const start = componentSource.indexOf(`@media (max-width: ${maxWidth}px)`)
  expect(start).toBeGreaterThanOrEqual(0)

  const nextMedia = componentSource.indexOf('@media ', start + 1)
  return componentSource.slice(start, nextMedia === -1 ? undefined : nextMedia)
}

describe('HeyRouteAuthLayout responsive grid', () => {
  it('uses the canonical Chinese locale code when switching from English', () => {
    expect(componentSource).toContain("setLocale(isChinese.value ? 'en' : 'zh')")
    expect(componentSource).not.toContain("setLocale(isChinese.value ? 'en' : 'zh-CN')")
  })

  it('keeps the register layout single-column below the tablet breakpoint', () => {
    const tabletStyles = mediaBlock(900)

    expect(tabletStyles).toMatch(
      /\.hr-auth-main,\s*\.is-register \.hr-auth-main\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/s,
    )
  })

  it('keeps the register layout inside the mobile viewport', () => {
    const mobileStyles = mediaBlock(640)

    expect(mobileStyles).toMatch(
      /\.hr-auth-main,\s*\.is-register \.hr-auth-main\s*{[^}]*width:\s*calc\(100% - 32px\)/s,
    )
  })

  it('uses component animations without document view-transition snapshots', () => {
    expect(componentSource).not.toContain('startViewTransition')
    expect(componentSource).not.toContain('view-transition-name')
    expect(componentSource).not.toContain('::view-transition')
    expect(componentSource).toContain('hr-auth-card-in')
  })

  it('falls back immediately while WebGL is resizing or unavailable', () => {
    expect(fluidSource).toContain('alpha: true')
    expect(fluidSource).toContain('preserveDrawingBuffer: false')
    expect(fluidSource).toContain("canvas.addEventListener('webglcontextlost', handleContextLost)")
    expect(fluidSource).toContain('isReady.value = false')
    expect(backdropSource).toContain('.hr-auth-backdrop__canvas:not(.ready)')
    expect(backdropSource).toContain('transition-duration: 0s')
  })
})
