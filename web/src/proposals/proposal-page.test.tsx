import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ProposalPage } from "./proposal-page"
import type { MergeConflict, Proposal } from "./queries"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "viewer-1" } }),
}))

const proposalId = "3f2c8d1e-0b5a-4c7d-9e11-2a6b8c4d5e70"
const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"

const ada = {
  kind: "user" as const,
  id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
  name: "Ada",
}

const clash: MergeConflict = {
  kind: "panel",
  id: "p-errors",
  reason: "both_changed",
  message: "Both you and the proposal changed Errors.",
}

function proposal(over: Partial<Proposal> = {}): Proposal {
  return {
    id: proposalId,
    dashboard_id: dashboardId,
    state: "open",
    title: "Tune the errors panel",
    revision: 2,
    base_version: 3,
    summary: "Raises the threshold",
    author: ada,
    origin: "web",
    origin_client: null,
    run_id: null,
    created_at: "2026-11-11T10:00:00Z",
    updated_at: "2026-11-11T10:00:00Z",
    closed_at: null,
    closed_by: null,
    merged_version: null,
    behind: true,
    latest_version: 4,
    allowed_actions: ["merge", "discard"],
    spec: {
      specVersion: "0",
      panels: [{ id: "p-errors", title: "Errors" }],
    },
    changes: [{ kind: "panel", id: "p-errors", change: "changed" }],
    conflicts: [clash],
    ...over,
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

type Reply = Response | (() => Response)

function mockServer(options: {
  proposals: Proposal[]
  merge?: Reply
  discard?: Reply
  status?: number
}) {
  const posts: { path: string; body: unknown }[] = []
  let reads = 0
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "POST") {
        posts.push({ path: pathname, body: await request.clone().json() })
        const reply = pathname.endsWith("/merge")
          ? options.merge
          : options.discard
        return (typeof reply === "function" ? reply() : reply) ?? json({}, 500)
      }
      if (pathname === `/api/v1/proposals/${proposalId}`) {
        if (options.status !== undefined) {
          return json({ error: "no", code: "not_found" }, options.status)
        }
        const index = Math.min(reads, options.proposals.length - 1)
        reads += 1
        return json(options.proposals[index])
      }
      if (pathname === `/api/v1/dashboards/${dashboardId}`) {
        return json({
          dashboard: { id: dashboardId, title: "Checkout" },
          version: { version: 4, spec: { specVersion: "0", panels: [] } },
        })
      }
      if (pathname === "/api/v1/threads") {
        return json({
          threads: [],
          messages: [],
          next_cursor: null,
          can_comment: true,
        })
      }
      return json({ error: "unexpected", code: "unexpected" }, 500)
    })
  )
  return posts
}

function renderPage() {
  return renderWithProviders(<ProposalPage proposalId={proposalId} />, {
    route: { path: "/", url: "/" },
  })
}

beforeEach(() => {
  toasts.success.mockClear()
  toasts.error.mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("ProposalPage", () => {
  it("shows the change by panel with its conflict and the state line", async () => {
    mockServer({ proposals: [proposal()] })
    renderPage()
    expect(
      await screen.findByRole("heading", { name: "Tune the errors panel" })
    ).toBeTruthy()
    expect(
      screen.getByText(
        "Open, based on version 3. The dashboard is now at version 4."
      )
    ).toBeTruthy()
    expect(screen.getByText("Errors")).toBeTruthy()
    expect(
      screen.getByText("Both you and the proposal changed Errors.")
    ).toBeTruthy()
    expect(screen.getByRole("radiogroup", { name: "Errors" })).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Open the dashboard at version 3" })
    ).toBeTruthy()
  })

  it("keeps Merge disabled until a side is picked, then sends the choice", async () => {
    const posts = mockServer({
      proposals: [proposal()],
      merge: json({ ...proposal(), state: "merged", merged_version: 5 }),
    })
    const { user } = renderPage()
    const merge = await screen.findByRole("button", { name: "Merge" })
    expect(merge.getAttribute("aria-disabled")).toBe("true")
    expect(merge.getAttribute("aria-describedby")).toBeTruthy()
    expect(
      screen.getByText("Choose a side for each conflict first")
    ).toBeTruthy()
    await user.click(merge)
    expect(posts).toEqual([])

    await user.click(
      screen.getByRole("radio", { name: "Take this proposal's" })
    )
    expect(merge.getAttribute("aria-disabled")).toBeNull()
    await user.click(merge)
    await waitFor(() =>
      expect(posts).toEqual([
        {
          path: `/api/v1/proposals/${proposalId}/merge`,
          body: {
            revision: 2,
            choices: [{ kind: "panel", id: "p-errors", take: "theirs" }],
          },
        },
      ])
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith(
        "Merged as version 5",
        expect.objectContaining({
          action: expect.objectContaining({ label: "Open dashboard" }),
        })
      )
    )
  })

  it("sends no choices when the proposal has no conflicts", async () => {
    const posts = mockServer({
      proposals: [proposal({ conflicts: [], behind: false })],
      merge: json({ ...proposal(), state: "merged", merged_version: 4 }),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Merge" }))
    await waitFor(() =>
      expect(posts).toEqual([
        {
          path: `/api/v1/proposals/${proposalId}/merge`,
          body: { revision: 2 },
        },
      ])
    )
  })

  it("blocks Merge and says why when a change cannot be merged", async () => {
    mockServer({
      proposals: [
        proposal({
          conflicts: [
            { ...clash, reason: "invalid", message: "Its query is invalid." },
          ],
        }),
      ],
    })
    renderPage()
    const merge = await screen.findByRole("button", { name: "Merge" })
    expect(merge.getAttribute("aria-disabled")).toBe("true")
    expect(
      screen.getByText(
        "Some changes cannot be merged; discard this proposal or ask its author to fix it"
      )
    ).toBeTruthy()
    expect(screen.queryByRole("radiogroup")).toBeNull()
    expect(
      screen.getByText("This change cannot be merged as it is")
    ).toBeTruthy()
  })

  it("shows no Merge when the caller may only discard", async () => {
    mockServer({ proposals: [proposal({ allowed_actions: ["discard"] })] })
    renderPage()
    expect(await screen.findByRole("button", { name: "Discard" })).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Merge" })).toBeNull()
  })

  it("shows the merged line and no action bar on a closed proposal", async () => {
    mockServer({
      proposals: [
        proposal({
          state: "merged",
          merged_version: 5,
          closed_by: { ...ada, name: "Grace" },
          allowed_actions: [],
          conflicts: [],
          behind: false,
        }),
      ],
    })
    renderPage()
    expect(await screen.findByText("Merged as version 5 by Grace")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Merge" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Discard" })).toBeNull()
  })

  it("asks before discarding and then discards", async () => {
    const posts = mockServer({
      proposals: [proposal({ conflicts: [] })],
      discard: json({ ...proposal(), state: "discarded" }),
    })
    const { user } = renderPage()
    await user.click(await screen.findByRole("button", { name: "Discard" }))
    const dialog = screen.getByRole("alertdialog")
    expect(within(dialog).getByText("Discard this proposal?")).toBeTruthy()
    expect(within(dialog).getByText("Its change is not saved.")).toBeTruthy()
    expect(posts).toEqual([])
    await user.click(within(dialog).getByRole("button", { name: "Discard" }))
    await waitFor(() =>
      expect(posts).toEqual([
        { path: `/api/v1/proposals/${proposalId}/discard`, body: {} },
      ])
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith("Discarded")
    )
  })

  it("reloads the proposal and drops picks for conflicts that are gone after a newer revision", async () => {
    const posts = mockServer({
      proposals: [
        proposal(),
        proposal({ revision: 3, conflicts: [], behind: false }),
      ],
      merge: json(
        { error: "newer", code: "proposal_changed", revision: 3 },
        409
      ),
    })
    const { user } = renderPage()
    await user.click(
      await screen.findByRole("radio", { name: "Keep the dashboard's" })
    )
    await user.click(screen.getByRole("button", { name: "Merge" }))
    expect(
      await screen.findByText(
        "This proposal has a newer revision. Review it and merge again."
      )
    ).toBeTruthy()
    await waitFor(() => expect(screen.queryByRole("radiogroup")).toBeNull())
    expect(posts).toHaveLength(1)
    await user.click(screen.getByRole("button", { name: "Merge" }))
    await waitFor(() => expect(posts).toHaveLength(2))
    expect(posts[1]?.body).toEqual({ revision: 3 })
  })

  it("says the proposal does not exist on a 404", async () => {
    mockServer({ proposals: [], status: 404 })
    renderPage()
    expect(
      await screen.findByText(
        "This proposal does not exist or you cannot see it"
      )
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Back to Dashboards" })
    ).toBeTruthy()
  })

  it("offers a retry on any other load failure", async () => {
    mockServer({ proposals: [], status: 500 })
    renderPage()
    expect(await screen.findByText("Could not load this proposal")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
  })
})
