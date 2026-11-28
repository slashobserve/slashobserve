import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { savedSearchKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { SearchDraft } from "./draft"
import { OpenSavedSearch } from "./open-saved-search"
import type { SavedSearch } from "./queries"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock("sonner", () => ({ toast: toasts }))

const id = "5f0c1e0e-0000-4000-8000-000000000001"
const folder = { id: "f1", title: "Shop", general: false, personal: false }
const permissions = { edit: true, comment: true, manage: false, create: true }

const logsSaved = {
  id,
  kind: "logs",
  title: "Checkout errors",
  datasource_uid: "es",
  query: "level:error",
  filters: { columns: ["host"] },
  from: "now-6h",
  to: "now",
  folder,
  permissions,
}

const logsDraft: SearchDraft = {
  kind: "logs",
  datasource_uid: "es",
  query: "level:error",
  filters: { columns: ["host"] },
  from: "now-6h",
  to: "now",
}

const tracesSaved = {
  ...logsSaved,
  kind: "traces",
  datasource_uid: "tempo",
  query: null,
  filters: { service: "checkout", errors: true },
}

const tracesDraft: SearchDraft = {
  kind: "traces",
  datasource_uid: "tempo",
  filters: { service: "checkout", errors: true },
  from: "now-6h",
  to: "now",
}

const metricsSaved = {
  ...logsSaved,
  kind: "metrics",
  datasource_uid: "prom",
  query: "up",
  filters: { mode: "table" },
}

const metricsDraft: SearchDraft = {
  kind: "metrics",
  datasource_uid: "prom",
  query: "up",
  filters: { mode: "table" },
  from: "now-6h",
  to: "now",
}

function setup({
  stored = logsSaved,
  savedId = id,
  uid = stored.datasource_uid,
  draft = logsDraft,
  mayEdit = true,
}: {
  stored?: Record<string, unknown> & { kind: string; datasource_uid: string }
  savedId?: string
  uid?: string
  draft?: SearchDraft | undefined
  mayEdit?: boolean
} = {}) {
  const onClose = vi.fn()
  const queryClient = testQueryClient()
  queryClient.setQueryData(
    savedSearchKeys.list({ kind: stored.kind as SavedSearch["kind"] }),
    {
      saved_searches: [
        { ...stored, permissions: { ...permissions, edit: mayEdit } },
      ],
      next_cursor: null,
    }
  )
  const view = renderWithProviders(
    <OpenSavedSearch
      kind={stored.kind as SavedSearch["kind"]}
      uid={uid}
      savedId={savedId}
      draft={draft}
      onClose={onClose}
    />,
    { queryClient }
  )
  return { ...view, onClose }
}

const updateButton = () =>
  screen.queryByRole("button", { name: "Update saved search" })

describe("OpenSavedSearch", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("names the saved search that is open and its folder", () => {
    setup()
    expect(screen.getByRole("status")).toHaveTextContent(
      "Saved search Checkout errors in Shop"
    )
    expect(screen.queryByText(/You changed it/)).toBeNull()
  })

  it("offers Update saved search once the query changed", () => {
    setup({ draft: { ...logsDraft, query: "level:warn" } })
    expect(screen.getByText(/You changed it/)).toBeVisible()
    expect(updateButton()).toBeVisible()
  })

  it("offers no update without Edit and says Save search makes a new one", () => {
    setup({ mayEdit: false, draft: { ...logsDraft, query: "level:warn" } })
    expect(screen.getByText(/this one stays as it was saved/)).toBeVisible()
    expect(updateButton()).toBeNull()
  })

  it("offers no update while nothing changed", () => {
    setup()
    expect(updateButton()).toBeNull()
  })

  it("offers no update when the page has nothing to save", () => {
    setup({ draft: undefined })
    expect(updateButton()).toBeNull()
  })

  it("sends the query, columns and range, and says so", async () => {
    const sent: unknown[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        sent.push(await request.clone().json())
        return Response.json({ ...logsSaved, query: "level:warn" })
      })
    )
    const draft = { ...logsDraft, query: "level:warn", from: "now-1h" }
    delete draft.filters
    const { user } = setup({
      draft,
    })
    await user.click(updateButton()!)
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Updated Checkout errors")
    )
    expect(sent).toEqual([
      { query: "level:warn", filters: null, from: "now-1h", to: "now" },
    ])
  })

  it("offers no update on a trace search that is as it was saved", () => {
    setup({ stored: tracesSaved, draft: tracesDraft })
    expect(updateButton()).toBeNull()
  })

  it("offers Update on a trace search whose filters changed", () => {
    setup({
      stored: tracesSaved,
      draft: { ...tracesDraft, filters: { service: "payments", errors: true } },
    })
    expect(updateButton()).toBeVisible()
  })

  it("clears the query of a trace search that no longer has one", async () => {
    const sent: unknown[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        sent.push(await request.clone().json())
        return Response.json(tracesSaved)
      })
    )
    const { user } = setup({
      stored: { ...tracesSaved, query: "{ status = error }" },
      draft: tracesDraft,
    })
    await user.click(updateButton()!)
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toMatchObject({ query: "" })
  })

  it("offers Update on a metrics search when its mode changed", () => {
    setup({ stored: metricsSaved, draft: metricsDraft })
    expect(updateButton()).toBeNull()
    const draft = { ...metricsDraft }
    delete draft.filters
    setup({ stored: metricsSaved, draft })
    expect(updateButton()).toBeVisible()
  })

  it("shows nothing without a saved search in the address", () => {
    setup({ savedId: "other" })
    expect(screen.queryByRole("status")).toBeNull()
  })

  it("shows nothing on another source than the one it was saved on", () => {
    setup({ uid: "os" })
    expect(screen.queryByRole("status")).toBeNull()
  })

  it("closes through the callback", async () => {
    const { user, onClose } = setup()
    await user.click(
      screen.getByRole("button", { name: "Close the saved search" })
    )
    expect(onClose).toHaveBeenCalled()
  })
})
