import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ActivityComposer, type Target } from "./activity-composer"
import type { AgentGate } from "./agent-gate"

const mutation = { mutate: vi.fn(), isPending: false }

vi.mock("@/observe/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/observe/queries")>()),
  useAgentInfo: () => ({
    data: {
      available: true,
      provider: "anthropic",
      model: "claude-haiku-5-5",
      effort: null,
    },
  }),
  useCreateObserveRequest: () => mutation,
}))

vi.mock("@/revisions/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/revisions/queries")>()),
  useRequestRevision: () => mutation,
}))

vi.mock("@/comments/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/comments/queries")>()),
  useAddComment: () => mutation,
}))

vi.mock("@/chat/prompt-history", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/chat/prompt-history")>()),
  usePromptHistory: () => ({ entries: [], record: vi.fn() }),
}))

const panels = [
  { id: "latency", title: "Latency" },
  { id: "errors", title: "Errors" },
]

function renderComposer({
  gate = { kind: "allowed" },
  draft = "Raise the threshold",
}: { gate?: AgentGate; draft?: string } = {}) {
  const onTargetChange = vi.fn<(target: Target) => void>()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ActivityComposer
        dashboardId="dashboard"
        latestVersion={3}
        panels={panels}
        target={{ kind: "dashboard" }}
        onTargetChange={onTargetChange}
        draft={draft}
        onDraftChange={vi.fn()}
        gate={gate}
        placeholder="Write a comment"
        range={{ from: "now-1h", to: "now" }}
      />
    </QueryClientProvider>
  )
  return {
    form: screen.getByRole("form", { name: "Add to the activity" }),
    onTargetChange,
  }
}

describe("ActivityComposer", () => {
  it("holds the target combobox, starting with the whole dashboard, and the mention picker", async () => {
    const user = userEvent.setup()
    const { form } = renderComposer()
    const comboboxes = within(form).getAllByRole("combobox")
    expect(comboboxes).toHaveLength(2)
    expect(comboboxes[0]).toHaveAccessibleName("On")
    expect(comboboxes[0]).toHaveTextContent("The whole dashboard")
    expect(comboboxes[1]).toHaveAccessibleName("Mention a person or an agent")
    await user.click(comboboxes[0])
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "The whole dashboard",
      "Latency",
      "Errors",
    ])
  })

  it("targets a panel when one is chosen", async () => {
    const user = userEvent.setup()
    const { form, onTargetChange } = renderComposer()
    await user.click(within(form).getByRole("combobox", { name: "On" }))
    await user.click(await screen.findByRole("option", { name: "Errors" }))
    expect(onTargetChange).toHaveBeenCalledWith({
      kind: "panel",
      panelId: "errors",
    })
  })

  it("offers both actions when the agent is allowed", () => {
    const { form } = renderComposer()
    expect(
      within(form).getByRole("button", { name: "Add comment" })
    ).toBeEnabled()
    expect(
      within(form).getByRole("button", { name: "Send to agent" })
    ).toBeEnabled()
  })

  it("offers only a comment and says why when the agent is blocked", () => {
    const reason = "Finish editing the layout to send changes to the agent."
    const { form } = renderComposer({ gate: { kind: "blocked", reason } })
    expect(
      within(form).getByRole("button", { name: "Add comment" })
    ).toBeVisible()
    expect(
      within(form).queryByRole("button", { name: "Send to agent" })
    ).not.toBeInTheDocument()
    expect(within(form).getByText(reason)).toBeVisible()
  })

  it("disables adding a comment while the draft is empty", () => {
    const { form } = renderComposer({ draft: "  " })
    expect(
      within(form).getByRole("button", { name: "Add comment" })
    ).toBeDisabled()
  })
})
