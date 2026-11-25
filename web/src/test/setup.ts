import "@testing-library/jest-dom/vitest"
import { cleanup } from "@testing-library/react"
import { afterEach } from "vitest"

import { useRevisionsAsked } from "@/conversation/revisions-asked"

if (typeof document !== "undefined" && !("fonts" in document)) {
  Object.defineProperty(document, "fonts", {
    value: { status: "loaded", ready: Promise.resolve() },
  })
}

if (typeof ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

afterEach(cleanup)
afterEach(() => useRevisionsAsked.setState(useRevisionsAsked.getInitialState()))
