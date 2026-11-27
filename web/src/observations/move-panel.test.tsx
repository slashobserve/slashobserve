import { createRef } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import type { Confined } from "./confined"
import { MovePanel } from "./move-panel"
import type { Observation } from "./queries"

const observation = {
  id: "o1",
  folder_id: "f1",
  title: "Payroll drift",
} as Observation

function render(confined?: Confined) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ folders: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
    )
  )
  const host = document.createElement("div")
  document.body.append(host)
  renderWithProviders(
    <PanelHostContext value={host}>
      <MovePanel
        observation={observation}
        confined={confined}
        trigger={createRef<HTMLElement>()}
        onClose={vi.fn()}
      />
    </PanelHostContext>
  )
  return host
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("MovePanel", () => {
  it("says before the move that a run in flight keeps the observation with Admins", () => {
    const host = render({ reason: "run_in_flight" })

    expect(host).toHaveTextContent(
      "An agent run on this observation has not finished. Until it ends, the observation can only move to a folder only Admins can read."
    )
  })

  it("says before the move that an answer keeps the observation with Admins", () => {
    const host = render({ reason: "permanent" })

    expect(host).toHaveTextContent(
      "This observation holds an answer that read data only Admins can query"
    )
  })

  it("shows no note when nothing confines the observation", () => {
    const host = render()

    expect(host).not.toHaveTextContent("only Admins")
  })
})
