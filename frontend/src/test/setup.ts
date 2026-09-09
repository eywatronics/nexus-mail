/**
 * jsdom reports every element as zero-sized, which makes a virtualiser
 * conclude that nothing is visible and render no rows at all. Fixing the
 * reported viewport is what lets the row-count assertions mean anything.
 */
const VIEWPORT_HEIGHT = 600
const VIEWPORT_WIDTH = 420

Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
  configurable: true,
  get: () => VIEWPORT_HEIGHT,
})

Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
  configurable: true,
  get: () => VIEWPORT_HEIGHT,
})

HTMLElement.prototype.getBoundingClientRect = function (): DOMRect {
  return {
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect
}

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = NoopResizeObserver as unknown as typeof ResizeObserver

if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia
}
