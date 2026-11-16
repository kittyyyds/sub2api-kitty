import { mount } from '@vue/test-utils'
import { defineComponent, h, shallowRef } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useHeyRouteSmoothScroll } from '../useHeyRouteSmoothScroll'

type MockLenisEvent = 'scroll' | 'virtual-scroll'
type MockLenisHandler = (payload: unknown) => void

interface MockLenisInstance {
  isScrolling: boolean | string
  raf: ReturnType<typeof vi.fn>
  destroy: ReturnType<typeof vi.fn>
  emit: (event: MockLenisEvent, payload?: unknown) => void
  on: (event: MockLenisEvent, callback: MockLenisHandler) => () => void
}

const lenisState = vi.hoisted(() => ({
  instances: [] as MockLenisInstance[],
}))

vi.mock('lenis', () => ({
  default: class MockLenis implements MockLenisInstance {
    isScrolling: boolean | string = false
    raf = vi.fn()
    destroy = vi.fn()
    private handlers = new Map<MockLenisEvent, Set<MockLenisHandler>>()

    constructor() {
      lenisState.instances.push(this)
    }

    on(event: MockLenisEvent, callback: MockLenisHandler): () => void {
      const callbacks = this.handlers.get(event) ?? new Set<MockLenisHandler>()
      callbacks.add(callback)
      this.handlers.set(event, callbacks)
      return () => callbacks.delete(callback)
    }

    emit(event: MockLenisEvent, payload: unknown = this): void {
      this.handlers.get(event)?.forEach((callback) => callback(payload))
    }
  },
}))

describe('useHeyRouteSmoothScroll', () => {
  let nextFrameId = 1
  let frames = new Map<number, FrameRequestCallback>()

  function flushFrame(time: number): void {
    const pending = frames.entries().next().value as [number, FrameRequestCallback] | undefined
    if (!pending) throw new Error('Expected a pending animation frame')
    frames.delete(pending[0])
    pending[1](time)
  }

  beforeEach(() => {
    nextFrameId = 1
    frames = new Map()
    lenisState.instances.length = 0
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
      matches: query === '(min-width: 901px)',
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      const frameId = nextFrameId
      nextFrameId += 1
      frames.set(frameId, callback)
      return frameId
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((frameId) => {
      frames.delete(frameId)
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('sleeps while idle and wakes for scrolling or page visibility', () => {
    const wrapper = mount(defineComponent({
      setup() {
        const rootRef = shallowRef<HTMLElement | null>(null)
        useHeyRouteSmoothScroll(rootRef)
        return () => h('div', { ref: rootRef })
      },
    }))

    const instance = lenisState.instances[0]
    expect(instance).toBeDefined()
    expect(frames.size).toBe(1)

    flushFrame(16)
    flushFrame(32)
    flushFrame(48)
    expect(frames.size).toBe(0)
    expect(instance.raf).toHaveBeenCalledTimes(3)

    instance.emit('virtual-scroll', {})
    expect(frames.size).toBe(1)
    instance.isScrolling = 'smooth'
    flushFrame(64)
    expect(frames.size).toBe(1)

    instance.isScrolling = false
    flushFrame(80)
    flushFrame(96)
    flushFrame(112)
    expect(frames.size).toBe(0)

    instance.emit('virtual-scroll', {})
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(frames.size).toBe(0)

    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(frames.size).toBe(1)

    wrapper.unmount()
    expect(frames.size).toBe(0)
    expect(instance.destroy).toHaveBeenCalledOnce()
  })

  it('stops when the window narrows to the mobile layout and restarts when it widens again', () => {
    let desktop = true
    const listeners = new Set<() => void>()
    vi.mocked(window.matchMedia).mockImplementation((query) => ({
      get matches() { return query === '(min-width: 901px)' ? desktop : false },
      media: query,
      onchange: null,
      addEventListener: (_type: string, listener: () => void) => { if (query === '(min-width: 901px)') listeners.add(listener) },
      removeEventListener: (_type: string, listener: () => void) => { listeners.delete(listener) },
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }) as unknown as MediaQueryList)

    const wrapper = mount(defineComponent({
      setup() {
        const rootRef = shallowRef<HTMLElement | null>(null)
        useHeyRouteSmoothScroll(rootRef)
        return () => h('div', { ref: rootRef })
      },
    }))
    expect(lenisState.instances).toHaveLength(1)

    desktop = false
    listeners.forEach((listener) => listener())
    expect(lenisState.instances[0].destroy).toHaveBeenCalledOnce()
    expect(frames.size).toBe(0)

    desktop = true
    listeners.forEach((listener) => listener())
    expect(lenisState.instances).toHaveLength(2)

    wrapper.unmount()
    expect(lenisState.instances[1].destroy).toHaveBeenCalledOnce()
    expect(listeners.size).toBe(0)
  })
})
