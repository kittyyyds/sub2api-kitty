import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const testDirectory = dirname(fileURLToPath(import.meta.url))
const toolbarSource = readFileSync(resolve(testDirectory, '../components/ChatToolbar.vue'), 'utf8')
const composerSource = readFileSync(resolve(testDirectory, '../components/ChatComposer.vue'), 'utf8')

describe('Kitty Chat responsive layout', () => {
  it('uses the project screen-reader utility for hidden native controls', () => {
    expect(toolbarSource).toContain('<span class="sr-only">API Key</span>')
    expect(toolbarSource).toContain('<span class="sr-only">模型</span>')
    expect(composerSource).toContain('class="sr-only"')
    expect(toolbarSource).not.toContain('visually-hidden')
    expect(composerSource).not.toContain('visually-hidden')
  })

  it('sizes selectors from the toolbar container instead of the viewport', () => {
    expect(toolbarSource).toContain('flex: 1 1 420px;')
    expect(toolbarSource).toContain('flex-wrap: wrap;')
    expect(toolbarSource).toContain('max-width: 100%;')
    expect(toolbarSource).not.toMatch(/width:\s*min\([^;]*vw/)
  })

  it('keeps the mobile toolbar compact without percentage-plus-gap overflow', () => {
    expect(toolbarSource).toContain('flex-wrap: nowrap;')
    expect(toolbarSource).toContain('justify-content: flex-start;')
    expect(toolbarSource).toContain('min-width: 0;')
    expect(toolbarSource).not.toContain('width: 50%;')
  })
})
