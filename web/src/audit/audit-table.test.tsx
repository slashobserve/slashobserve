import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AuditTable, auditColumns } from "./audit-table"
import type { AuditEvent } from "./stream"
import { useBesideDetail } from "./use-beside-detail"

vi.mock("./use-beside-detail", () => ({ useBesideDetail: vi.fn(() => false) }))

function auditEvent(id: string, action: string): AuditEvent {
  return {
    id,
    occurred_at: "2026-11-21T06:00:00Z",
    action,
    outcome: "success",
    reason: null,
    actor_kind: "user",
    actor_name: "Ada",
    origin: "web",
    origin_client: null,
    target_name: "Checkout",
    target_id: null,
  } as unknown as AuditEvent
}

const event = auditEvent("e1", "dashboard.update")

afterEach(() => vi.mocked(useBesideDetail).mockReturnValue(false))

function headers() {
  return screen
    .getAllByRole("columnheader")
    .map((header) => header.textContent?.trim())
}

describe("auditColumns", () => {
  const ids = (beside: boolean) =>
    auditColumns(null, beside).map((column) => column.id)

  it("keeps every column while the detail panel is not beside the table", () => {
    expect(ids(false)).toEqual([
      "action",
      "time",
      "actor",
      "outcome",
      "target",
      "origin",
    ])
  })

  it("drops Target and Origin while the detail panel is beside the table", () => {
    expect(ids(true)).toEqual(["action", "time", "actor", "outcome"])
  })

  it("leaves a missing target out of the cell so a stacked card hides it", () => {
    const target = auditColumns(null, false).find(
      (column) => column.id === "target"
    )
    expect(target?.cell({ ...event, target_name: null } as AuditEvent)).toBe(
      null
    )
  })
})

describe("AuditTable", () => {
  it("shows every column and marks the selected event", async () => {
    render(
      <AuditTable
        events={[event, auditEvent("e2", "auth.login")]}
        selected="e2"
        panelOpen={false}
        onSelect={vi.fn()}
      />
    )
    await screen.findByRole("gridcell", { name: "dashboard.update" })
    expect(headers()).toEqual([
      "Action",
      "Time",
      "Actor",
      "Outcome",
      "Target",
      "Origin",
    ])
    expect(screen.getByRole("button", { name: "auth.login" })).toHaveAttribute(
      "aria-current",
      "true"
    )
    expect(
      screen.getByRole("button", { name: "dashboard.update" })
    ).not.toHaveAttribute("aria-current")
  })

  it("drops Target and Origin only while the panel is open beside the table", async () => {
    vi.mocked(useBesideDetail).mockReturnValue(true)
    const { rerender } = render(
      <AuditTable events={[event]} selected="e1" panelOpen onSelect={vi.fn()} />
    )
    await screen.findByRole("gridcell", { name: "dashboard.update" })
    expect(headers()).toEqual(["Action", "Time", "Actor", "Outcome"])
    rerender(
      <AuditTable
        events={[event]}
        selected={null}
        panelOpen={false}
        onSelect={vi.fn()}
      />
    )
    await waitFor(() => expect(headers()).toHaveLength(6))
  })

  it("selects an event from its action", async () => {
    const onSelect = vi.fn()
    render(
      <AuditTable
        events={[event]}
        selected={null}
        panelOpen={false}
        onSelect={onSelect}
      />
    )
    fireEvent.click(
      await screen.findByRole("button", { name: "dashboard.update" })
    )
    expect(onSelect).toHaveBeenCalledWith("e1")
  })

  it("adds a live event at the top without redrawing the rows already there", async () => {
    const first = [event]
    const { rerender } = render(
      <AuditTable
        events={first}
        selected={null}
        panelOpen={false}
        onSelect={vi.fn()}
      />
    )
    const before = (
      await screen.findByRole("button", { name: "dashboard.update" })
    ).closest("[role=row]")
    rerender(
      <AuditTable
        events={[auditEvent("live", "auth.login"), ...first]}
        selected={null}
        panelOpen={false}
        onSelect={vi.fn()}
      />
    )
    await screen.findByRole("button", { name: "auth.login" })
    const rows = screen
      .getAllByRole("row")
      .filter((row) => row.hasAttribute("row-id"))
      .sort(
        (a, b) =>
          Number(a.getAttribute("row-index")) -
          Number(b.getAttribute("row-index"))
      )
    expect(rows.map((row) => row.getAttribute("row-id"))).toEqual([
      "live",
      "e1",
    ])
    expect(rows[1]).toBe(before)
  })
})
