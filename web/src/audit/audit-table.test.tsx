import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { AuditTable, hiddenBesideDetail } from "./audit-table"
import type { AuditEvent } from "./stream"

const event = {
  id: "e1",
  occurred_at: "2026-11-21T06:00:00Z",
  action: "dashboard.update",
  outcome: "success",
  reason: null,
  actor_kind: "user",
  actor_name: "Ada",
  origin: "web",
  origin_client: null,
  target_name: "Checkout",
  target_id: null,
} as unknown as AuditEvent

function renderTable(panelOpen: boolean) {
  render(
    <AuditTable
      events={[event]}
      selected={panelOpen ? "e1" : null}
      panelOpen={panelOpen}
      onSelect={vi.fn()}
    />
  )
}

describe("AuditTable beside the detail panel", () => {
  it("keeps every column while no panel is open", () => {
    renderTable(false)
    expect(
      screen.getByRole("columnheader", { name: "Target" })
    ).not.toHaveClass(hiddenBesideDetail)
    expect(screen.getByText("Checkout")).not.toHaveClass(hiddenBesideDetail)
  })

  it("drops Target and Origin to a defined narrower set while the panel is beside it", () => {
    renderTable(true)
    for (const name of ["Target", "Origin"]) {
      expect(screen.getByRole("columnheader", { name })).toHaveClass(
        hiddenBesideDetail
      )
    }
    expect(screen.getByText("Checkout")).toHaveClass(hiddenBesideDetail)
    expect(screen.getByRole("columnheader", { name: "Actor" })).not.toHaveClass(
      hiddenBesideDetail
    )
  })
})
