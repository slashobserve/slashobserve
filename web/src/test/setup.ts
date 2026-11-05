import "@testing-library/jest-dom/vitest"
import { cleanup } from "@testing-library/react"
import { afterEach } from "vitest"

if (!("fonts" in document)) {
  Object.defineProperty(document, "fonts", {
    value: { status: "loaded", ready: Promise.resolve() },
  })
}

afterEach(cleanup)
