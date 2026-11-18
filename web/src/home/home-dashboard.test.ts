import { QueryClient } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi } from "vitest"

import { homeDashboardKeys } from "@/api/keys"

import {
  forgetHomeChecked,
  homeDashboardHref,
  markHomeChecked,
} from "./home-dashboard"

function clientWith(home: unknown) {
  const client = new QueryClient()
  client.setQueryData(homeDashboardKeys.current(), home, {
    updatedAt: Date.now(),
  })
  return client
}

const ok = {
  status: "ok",
  dashboard: {
    id: "d1",
    title: "Board",
    folder_id: "f",
    folder_title: "General",
  },
  set_at: "2026-11-19T00:00:00Z",
}

afterEach(() => {
  forgetHomeChecked()
  vi.restoreAllMocks()
})

describe("homeDashboardHref", () => {
  it("opens the chosen dashboard on the first visit to Home", async () => {
    expect(await homeDashboardHref(clientWith(ok), "")).toBe("/d/d1")
  })

  it("lets Home open when the tab has already been through Home or another page", async () => {
    await homeDashboardHref(clientWith(ok), "")
    expect(await homeDashboardHref(clientWith(ok), "")).toBeNull()
    forgetHomeChecked()
    markHomeChecked()
    expect(await homeDashboardHref(clientWith(ok), "")).toBeNull()
  })

  it("opens Home again after the flag is forgotten, as after signing in", async () => {
    await homeDashboardHref(clientWith(ok), "")
    forgetHomeChecked()
    expect(await homeDashboardHref(clientWith(ok), "")).toBe("/d/d1")
  })

  it("keeps Home for a link that names a tab", async () => {
    expect(await homeDashboardHref(clientWith(ok), "?tab=observe")).toBeNull()
  })

  it.each(["none", "gone", "unreadable"])(
    "keeps Home when the status is %s",
    async (status) => {
      const home = { status, dashboard: null, set_at: null }
      expect(await homeDashboardHref(clientWith(home), "")).toBeNull()
    }
  )

  it("keeps Home when the choice cannot be read", async () => {
    const client = new QueryClient()
    vi.spyOn(client, "fetchQuery").mockRejectedValue(new Error("offline"))
    expect(await homeDashboardHref(client, "")).toBeNull()
  })
})
