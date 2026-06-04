import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// Node v25+ ships --localstorage-file which, when passed without a valid path
// (as the Claude Code harness does), replaces globalThis.localStorage with a
// broken stub that has no methods. Restore a proper in-memory implementation
// so all tests that touch localStorage work correctly.
;(function patchLocalStorage() {
  const store: Record<string, string> = {}
  const impl: Storage = {
    get length() { return Object.keys(store).length },
    key(index: number) { return Object.keys(store)[index] ?? null },
    getItem(key: string) { return Object.prototype.hasOwnProperty.call(store, key) ? store[key] : null },
    setItem(key: string, value: string) { store[key] = String(value) },
    removeItem(key: string) { delete store[key] },
    clear() { for (const k of Object.keys(store)) delete store[k] },
  }
  // Only patch if the existing localStorage is broken (missing clear).
  if (typeof globalThis.localStorage?.clear !== 'function') {
    Object.defineProperty(globalThis, 'localStorage', {
      value: impl,
      writable: true,
      configurable: true,
    })
  }
})()

// cmdk (and other layout-aware libs) use ResizeObserver which jsdom doesn't provide.
// Stub it so tests don't throw "ResizeObserver is not defined".
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

// cmdk calls scrollIntoView on list items which jsdom doesn't implement.
if (typeof Element.prototype.scrollIntoView !== 'function') {
  Element.prototype.scrollIntoView = function () {}
}

// Auto-unmount components between tests so the jsdom document stays clean.
afterEach(() => cleanup())
