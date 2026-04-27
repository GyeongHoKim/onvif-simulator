import "@testing-library/jest-dom/vitest"
import { afterEach, vi } from "vitest"
import { cleanup } from "@testing-library/react"

import { appMocks, runtimeMocks, resetWailsMocks } from "./wails-mock"

// Global mock for Wails-generated modules. Tests that need a different
// behaviour can override individual functions on appMocks or runtimeMocks.
vi.mock("@/lib/wails/wailsjs/go/gui/App", () => appMocks)
vi.mock("@/lib/wails/wailsjs/runtime/runtime", () => runtimeMocks)

afterEach(() => {
  cleanup()
  resetWailsMocks()
})

if (typeof window !== "undefined" && !window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }),
  })
}

// Polyfill ResizeObserver for shadcn primitives that probe layout.
if (typeof window !== "undefined" && !window.ResizeObserver) {
  class RO {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  ;(window as unknown as { ResizeObserver: typeof RO }).ResizeObserver = RO
}
