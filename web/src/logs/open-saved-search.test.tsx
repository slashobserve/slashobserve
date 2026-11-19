import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { savedSearchKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { OpenSavedSearch } from "./open-saved-search"
import type { LogsSearch } from "./search"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

const id = "5f0c1e0e-0000-4000-8000-000000000001"

const saved = {
  id,
  kind: "logs",
  title: "Checkout errors",
  datasource_uid: "es",
  query: "level:error",
  filters: { columns: ["host"] },
  from: "now-6h",
  to: "now",
  folder: { id: "f1", title: "Shop", general: false, personal: false },
}

const unchanged = {
  draft: {
    kind: "logs" as const,
    datasource_uid: "es",
    query: "level:error",
    filters: { columns: ["host"] },
    from: "now-6h",
    to: "now",
  },
}

function setup({
  search = { saved: id },
  uid = "es",
  savable = unchanged,
}: {
  search?: LogsSearch
  uid?: string
  savable?: Parameters<typeof OpenSavedSearch>[0]["savable"]
} = {}) {
  const update = vi.fn()
  const queryClient = testQueryClient()
  queryClient.setQueryData(savedSearchKeys.list({ kind: "logs" }), {
    saved_searches: [saved],
    next_cursor: null,
  })
  const workspace = {
    datasource: { uid, type: "elasticsearch", name: uid },
    search,
    update,
  } as unknown as LogsWorkspace
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <OpenSavedSearch savable={savable} />
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { ...view, update }
}

describe("OpenSavedSearch", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("names the saved search that is open and its folder", () => {
    setup()
    expect(screen.getByRole("status")).toHaveTextContent(
      "Saved search Checkout errors in Shop"
    )
    expect(screen.queryByText(/You changed it/)).toBeNull()
  })

  it("says Save search makes a new search once the query changed", () => {
    setup({
      savable: {
        draft: { ...unchanged.draft, query: "level:warn" },
      },
    })
    expect(screen.getByText(/You changed it/)).toBeVisible()
  })

  it("shows nothing without a saved search in the address", () => {
    setup({ search: {} })
    expect(screen.queryByRole("status")).toBeNull()
  })

  it("shows nothing on another source than the one it was saved on", () => {
    setup({ uid: "os" })
    expect(screen.queryByRole("status")).toBeNull()
  })

  it("closes by leaving it out of the address", async () => {
    const { user, update } = setup()
    await user.click(
      screen.getByRole("button", { name: "Close the saved search" })
    )
    expect(update).toHaveBeenCalledWith({ saved: undefined }, { replace: true })
  })
})
