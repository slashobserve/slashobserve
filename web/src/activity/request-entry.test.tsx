import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { Revision } from "@/revisions/queries"
import { renderWithProviders } from "@/test/render"

import { RequestEntry } from "./request-entry"

function revision(patch: Partial<Revision>): Revision {
  return {
    id: "r1",
    dashboard_id: "d1",
    allowed_actions: [],
    intent: "apply",
    investigation: false,
    comment_id: null,
    panel_id: null,
    prompt: "Add a p99 panel",
    base_version: 1,
    status: "succeeded",
    state: "succeeded",
    result_kind: "none",
    result_version: null,
    error: null,
    requested_by: { kind: "user", id: "u2", name: "Grace" },
    created_at: "2026-11-21T10:00:00Z",
    started_at: null,
    finished_at: null,
    redacted: false,
    ...patch,
  } as Partial<Revision> as Revision
}

function show(request: Revision) {
  return renderWithProviders(
    <ol>
      <RequestEntry
        dashboardId="d1"
        entry={{
          kind: "request",
          key: `request:${request.id}`,
          at: request.created_at,
          panelId: null,
          request,
        }}
        panelTitle={null}
        onSelectPanel={() => {}}
      />
    </ol>,
    { route: { path: "/", url: "/" } }
  )
}

describe("RequestEntry", () => {
  it("shows what was asked", async () => {
    show(revision({}))
    expect(await screen.findByText("Add a p99 panel")).toBeVisible()
  })

  it("says a run from someone's private chat was asked there, in place of its words", async () => {
    show(revision({ prompt: "", redacted: true }))
    expect(await screen.findByText("Asked in a private chat")).toBeVisible()
  })
})
