import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'

import HeyRouteSupportDialog from '../HeyRouteSupportDialog.vue'
import { heyRouteCopyByLanguage } from '../heyRouteCopy'

const wrappers: VueWrapper[] = []
const opener = document.createElement('button')
opener.textContent = 'open support'

function mountDialog(): VueWrapper {
  document.body.appendChild(opener)
  const wrapper = mount(HeyRouteSupportDialog, {
    attachTo: document.body,
    props: { open: false, copy: heyRouteCopyByLanguage.en.support },
    // The shared setup clears VTU's default stubs; real leave transitions never finish in jsdom.
    global: { stubs: { Icon: true, transition: true } },
  })
  wrappers.push(wrapper)
  return wrapper
}

async function flush(): Promise<void> {
  await nextTick()
  await nextTick()
}

function dialogElement(): HTMLElement {
  const dialog = document.querySelector<HTMLElement>('.hr-contact-dialog')
  if (!dialog) throw new Error('dialog not rendered')
  return dialog
}

function pressKey(key: string, shiftKey = false): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, cancelable: true }))
}

describe('HeyRouteContactDialog focus handling', () => {
  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    opener.remove()
  })

  it('moves focus into the dialog on open and back to the opener on close', async () => {
    const wrapper = mountDialog()
    opener.focus()

    await wrapper.setProps({ open: true })
    await flush()
    expect(dialogElement().contains(document.activeElement)).toBe(true)

    await wrapper.setProps({ open: false })
    await flush()
    expect(document.activeElement).toBe(opener)
  })

  it('asks to close on Escape', async () => {
    const wrapper = mountDialog()
    opener.focus()
    await wrapper.setProps({ open: true })
    await flush()

    pressKey('Escape')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('keeps Tab focus inside the dialog', async () => {
    const wrapper = mountDialog()
    opener.focus()
    await wrapper.setProps({ open: true })
    await flush()

    const focusable = Array.from(dialogElement().querySelectorAll<HTMLElement>('a[href], button:not([disabled])'))
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    expect(first).toBeDefined()
    expect(last).toBeDefined()

    last?.focus()
    pressKey('Tab')
    expect(document.activeElement).toBe(first)

    first?.focus()
    pressKey('Tab', true)
    expect(document.activeElement).toBe(last)
  })
})
