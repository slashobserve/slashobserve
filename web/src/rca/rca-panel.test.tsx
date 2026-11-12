import { createRef } from "react"
import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { observationKeys } from "@/api/keys"
import type { Observation } from "@/observations/queries"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { RcaPanel } from "./rca-panel"

const observationId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"

const actor = {
  kind: "user",
  id: "u1",
  name: "Priya",
  origin: "web",
  client: null,
  run_id: null,
  label: "Priya",
}

function summary(revision: number) {
  return {
    id: `r${revision}`,
    revision,
    title: "Checkout outage",
    summary: null,
    observation_state: "open",
    created_by: actor,
    created_at: new Date().toISOString(),
  }
}

function observation(actions: string[]): Observation {
  return { id: observationId, allowed_actions: actions } as Observation
}

function renderPanel(actions: string[], revisions: number[]) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observationKeys.rcas(observationId), {
    rcas: revisions.map(summary),
  })
  const host = document.createElement("div")
  document.body.append(host)
  return renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <RcaPanel
        observation={observation(actions)}
        trigger={createRef()}
        onClose={() => {}}
      />
    </PanelHostContext.Provider>,
    { queryClient }
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("RcaPanel", () => {
  it("offers the next revision number to someone who may publish", async () => {
    renderPanel(["publish"], [2, 1])
    expect(
      await screen.findByRole("button", { name: "Publish revision 3" })
    ).toBeTruthy()
    expect(screen.getByText("Revision 2")).toBeTruthy()
    expect(screen.getByText("Revision 1")).toBeTruthy()
  })

  it("starts at revision 1 and says nothing is published yet", async () => {
    renderPanel(["publish"], [])
    expect(
      await screen.findByRole("button", { name: "Publish revision 1" })
    ).toBeTruthy()
    expect(screen.getByText("Not published yet.")).toBeTruthy()
  })

  it("only lists revisions for someone who may not publish", async () => {
    renderPanel(["update"], [1])
    expect(
      await screen.findByText("You can view revisions. Editors publish them.")
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: /^Publish/ })).toBeNull()
  })

  it("says why a publish was refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: "limit", code: "rca_limit" }), {
            status: 409,
            headers: { "Content-Type": "application/json" },
          })
        )
      )
    )
    const { user } = renderPanel(["publish"], [1])
    await user.click(
      await screen.findByRole("button", { name: "Publish revision 2" })
    )
    await waitFor(() =>
      expect(
        screen.getByText(
          "This observation has 50 revisions, the most it can have."
        )
      ).toBeTruthy()
    )
  })
})
