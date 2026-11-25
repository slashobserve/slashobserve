import { screen } from "@testing-library/react"
import { QueryClientProvider } from "@tanstack/react-query"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { DiscussionFilter } from "@/discussion/discussion-filter"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { DiscussionPane } from "./discussion-pane"
import type { Observation } from "./queries"

type FeedProps = {
  focusedThreadId?: string
  view?: { filter: DiscussionFilter; onClear: () => void }
}

vi.mock("@/discussion/discussion", () => ({
  BlockSelect: ({
    label,
    onChange,
  }: {
    label: string
    onChange: (blockId: string | null) => void
  }) => (
    <button type="button" onClick={() => onChange("b1")}>
      Pick {label}
    </button>
  ),
  DiscussionFeed: ({ focusedThreadId, view }: FeedProps) => (
    <div
      data-testid="feed"
      data-focused-thread={focusedThreadId}
      data-block={view?.filter.blockId ?? ""}
    />
  ),
}))

const observation = {
  id: "o1",
  updated_at: "2026-11-12T10:00:00Z",
  blocks: [
    { id: "b1", position: 0, kind: "text", body: {} },
    { id: "b2", position: 1, kind: "text", body: {} },
  ],
} as unknown as Observation

const host = document.createElement("div")

beforeEach(() => {
  document.body.append(host)
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(null, { status: 599 }))
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  host.remove()
})

function pane(props: { hidden: boolean; focusedThreadId?: string }) {
  return (
    <PanelHostContext value={host}>
      <DiscussionPane
        observation={observation}
        folderTitle="Incidents"
        canAdd={false}
        canEdit={false}
        headingRef={{ current: null }}
        onClose={() => {}}
        onShowBlock={() => {}}
        onAdded={() => {}}
        {...props}
      />
    </PanelHostContext>
  )
}

function show(
  queryClient: Parameters<typeof QueryClientProvider>[0]["client"],
  props: { hidden: boolean; focusedThreadId?: string }
) {
  return (
    <QueryClientProvider client={queryClient}>
      {pane(props)}
    </QueryClientProvider>
  )
}

describe("DiscussionPane focused thread", () => {
  it("passes the thread to the feed while shown", async () => {
    renderWithProviders(pane({ hidden: false, focusedThreadId: "t1" }))
    expect((await screen.findByTestId("feed")).dataset.focusedThread).toBe("t1")
  })

  it("hands the thread to the feed only once the pane is shown again", async () => {
    const { rerender, queryClient } = renderWithProviders(
      pane({ hidden: true, focusedThreadId: "t1" })
    )
    const feed = await screen.findByTestId("feed")
    expect(feed.dataset.focusedThread).toBeUndefined()

    rerender(show(queryClient, { hidden: false, focusedThreadId: "t1" }))
    expect(screen.getByTestId("feed").dataset.focusedThread).toBe("t1")
  })

  it("clears the block filter when a new thread is focused", async () => {
    const { user, rerender, queryClient } = renderWithProviders(
      pane({ hidden: false })
    )
    await user.click(await screen.findByRole("button", { name: "Pick Block" }))
    expect(screen.getByTestId("feed").dataset.block).toBe("b1")

    rerender(show(queryClient, { hidden: false, focusedThreadId: "t1" }))
    expect(screen.getByTestId("feed").dataset.block).toBe("")
  })

  it("keeps the block filter when the thread is dropped", async () => {
    const { user, rerender, queryClient } = renderWithProviders(
      pane({ hidden: false, focusedThreadId: "t1" })
    )
    await user.click(await screen.findByRole("button", { name: "Pick Block" }))

    rerender(show(queryClient, { hidden: false }))
    expect(screen.getByTestId("feed").dataset.block).toBe("b1")
  })
})
