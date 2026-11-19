import { createRef } from "react"
import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { observationKeys } from "@/api/keys"
import type { Block, Observation } from "@/observations/queries"
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

function renderPanel(
  actions: string[],
  revisions: number[],
  privateFolder?: () => void,
  blocks: Block[] = [],
  onShowBlock?: (blockId: string) => void
) {
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
        privateFolder={privateFolder}
        blocks={blocks}
        onShowBlock={onShowBlock}
      />
    </PanelHostContext.Provider>,
    { queryClient, route: { path: "/", url: "/" } }
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

  it("sends a private observation to a shared folder before publishing", async () => {
    const move = vi.fn()
    const { user } = renderPanel(["update", "move"], [], move)
    expect(
      await screen.findByText(/private to you in My Dashboards/)
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: /^Publish/ })).toBeNull()
    await user.click(screen.getByRole("button", { name: "Move to folder" }))
    expect(move).toHaveBeenCalledOnce()
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

  it("names the block that could not be frozen and shows it on request", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              error: "a panel failed",
              code: "panel_failed",
              issues: [
                {
                  path: "/blocks/1",
                  code: "query_timeout",
                  message: "ran out of time",
                },
              ],
            }),
            { status: 422, headers: { "Content-Type": "application/json" } }
          )
        )
      )
    )
    const blocks = [
      {
        id: "b1",
        position: 1,
        kind: "panel",
        body: { panel: { title: "Checkout latency" } },
      },
    ] as unknown as Block[]
    const show = vi.fn()
    const { user } = renderPanel(["publish"], [], undefined, blocks, show)
    await user.click(
      await screen.findByRole("button", { name: "Publish revision 1" })
    )
    expect(
      await screen.findByText(
        'The panel "Checkout latency" could not be frozen: ran out of time'
      )
    ).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "Show block" }))
    expect(show).toHaveBeenCalledWith("b1")
  })

  it("opens a revision and lets an editor withdraw it once it is selected", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ public_links: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        )
      )
    )
    const { user } = renderPanel(["publish"], [2, 1])
    expect(screen.queryByRole("button", { name: "Withdraw" })).toBeNull()
    await user.click(await screen.findByRole("button", { name: /Revision 1/ }))
    expect(
      await screen.findByRole("link", { name: "Open revision 1" })
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "Withdraw" })).toBeTruthy()
  })

  it("says once that a person who may not manage links cannot, with no way to create or retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              error: "your role does not allow this",
              code: "forbidden_role",
              needed_access: "manage",
              folder_title: "Payments",
            }),
            { status: 403, headers: { "Content-Type": "application/json" } }
          )
        )
      )
    )
    const { user } = renderPanel(["publish"], [1])
    await user.click(await screen.findByRole("button", { name: /Revision 1/ }))
    expect(
      await screen.findByText(
        "Your role does not allow this; it needs Manage on Payments."
      )
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Create link" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull()
  })

  it("lets someone who may not publish open a revision but not withdraw it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ public_links: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        )
      )
    )
    const { user } = renderPanel(["update"], [1])
    await user.click(await screen.findByRole("button", { name: /Revision 1/ }))
    expect(
      await screen.findByRole("link", { name: "Open revision 1" })
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Withdraw" })).toBeNull()
  })

  it("heads the panel of someone who may not publish as RCA revisions", async () => {
    renderPanel(["update"], [1])
    expect(
      await screen.findByRole("heading", { name: "RCA revisions" })
    ).toBeTruthy()
    expect(screen.queryByText("Publish as RCA")).toBeNull()
  })
})
