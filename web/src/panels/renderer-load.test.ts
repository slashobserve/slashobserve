import { describe, expect, it } from "vitest"

import { guardLoad, RendererLoadError } from "./renderer-load"

describe("guardLoad", () => {
  it("passes a loaded module through", async () => {
    await expect(
      guardLoad(() => Promise.resolve({ default: 1 }))()
    ).resolves.toEqual({
      default: 1,
    })
  })

  it("marks a failed load so the panel boundary reloads the page", async () => {
    const failure = new Error("Failed to fetch dynamically imported module")
    const load = guardLoad(() => Promise.reject(failure))
    const error = await load().catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(RendererLoadError)
    expect((error as RendererLoadError).message).toBe(failure.message)
    expect((error as RendererLoadError).cause).toBe(failure)
  })
})
