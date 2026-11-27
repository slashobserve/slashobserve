import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

const signedIn = {
  authenticated: true,
  principal: { kind: "user", id: "u1", name: "Ana", role: "editor" },
}
const signedOut = { authenticated: false }

let responses: { status: number; body: unknown }[] = []

beforeEach(() => {
  vi.resetModules()
  responses = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      const next = responses.shift()
      if (!next) throw new Error("no response queued")
      return new Response(JSON.stringify(next.body), {
        status: next.status,
        headers: { "content-type": "application/json" },
      })
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("sessionEnded", () => {
  it("is false for a visitor who never had a session", async () => {
    const auth = await import("./auth")
    responses.push({ status: 200, body: signedOut })
    await auth.getSession()
    expect(auth.sessionEnded()).toBe(false)
  })

  it("is true once a known session turns out to be gone, and clears on sign-in", async () => {
    const auth = await import("./auth")
    responses.push({ status: 200, body: signedIn })
    await auth.getSession()
    auth.invalidateSession()
    responses.push({ status: 200, body: signedOut })
    await auth.getSession()
    expect(auth.sessionEnded()).toBe(true)
    responses.push({ status: 200, body: signedIn })
    await auth.refreshSession()
    expect(auth.sessionEnded()).toBe(false)
  })

  it("is false after leaving on purpose without the sign-out request", async () => {
    const auth = await import("./auth")
    responses.push({ status: 200, body: signedIn })
    await auth.getSession()
    auth.leaveSession()
    responses.push({ status: 200, body: signedOut })
    await auth.getSession()
    expect(auth.sessionEnded()).toBe(false)
    expect(auth.signedInPrincipalId()).toBeUndefined()
  })

  it("keeps the signed-in principal while a check fails", async () => {
    const auth = await import("./auth")
    responses.push({ status: 200, body: signedIn })
    await auth.getSession()
    auth.invalidateSession()
    responses.push({ status: 500, body: null })
    await expect(auth.getSession()).rejects.toThrow()
    expect(auth.signedInPrincipalId()).toBe("u1")
    expect(auth.sessionEnded()).toBe(false)
  })

  it("is false after signing out on purpose", async () => {
    const auth = await import("./auth")
    responses.push({ status: 200, body: signedIn })
    await auth.getSession()
    responses.push({ status: 204, body: null })
    await auth.logout()
    auth.invalidateSession()
    responses.push({ status: 200, body: signedOut })
    await auth.getSession()
    expect(auth.sessionEnded()).toBe(false)
  })
})
