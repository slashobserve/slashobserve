import { fireEvent, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { AuditDetail } from "./audit-detail"
import type { AuditEvent } from "./stream"

const fetchMock = vi.hoisted(() => vi.fn())

function auditEvent(fields: Partial<AuditEvent> = {}): AuditEvent {
  return {
    id: "e2",
    seq: 2,
    occurred_at: "2026-11-21T06:00:00Z",
    category: "content",
    action: "dashboard.update",
    outcome: "success",
    reason: null,
    actor_kind: "user",
    actor_id: "11111111-1111-4111-8111-111111111111",
    actor_name: "Ada",
    initiator: "person",
    origin: "web",
    origin_client: null,
    request_id: null,
    run_id: null,
    target_kind: "dashboard",
    target_id: "22222222-2222-4222-8222-222222222222",
    target_name: "Checkout",
    data: {},
    ...fields,
  } as AuditEvent
}

function serve(event: AuditEvent) {
  fetchMock.mockImplementation(() =>
    Promise.resolve(
      new Response(JSON.stringify({ event, same_request: [], same_run: [] }), {
        headers: { "content-type": "application/json" },
      })
    )
  )
}

function open(
  event: AuditEvent,
  handlers: { onSelect?: () => void; onQuery?: () => void } = {}
) {
  serve(event)
  const host = document.createElement("div")
  document.body.append(host)
  const onSelect = handlers.onSelect ?? vi.fn()
  const onQuery = handlers.onQuery ?? vi.fn()
  const view = renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <AuditDetail
        id={event.id}
        neighbors={{ previous: "e1", next: "e3" }}
        onSelect={onSelect}
        onQuery={onQuery}
        onClose={vi.fn()}
      />
    </PanelHostContext.Provider>,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, onSelect, onQuery }
}

beforeEach(() => {
  document.body.replaceChildren()
  fetchMock.mockReset()
  vi.stubGlobal("fetch", fetchMock)
  return () => vi.unstubAllGlobals()
})

describe("AuditDetail", () => {
  it("links the target to its page", async () => {
    open(auditEvent())
    const link = await screen.findByRole("link", { name: "Checkout" })
    expect(link).toHaveAttribute(
      "href",
      "/d/22222222-2222-4222-8222-222222222222/checkout"
    )
  })

  it.each([
    ["a kind with no page", { target_kind: "datasource" }],
    ["a target the event deleted", { action: "dashboard.delete" }],
  ])("shows %s as plain text", async (_name, fields) => {
    open(auditEvent(fields))
    await screen.findByText("Checkout")
    expect(screen.queryByRole("link", { name: "Checkout" })).toBeNull()
  })

  it("filters the list to the actor or the target", async () => {
    const { user, onQuery } = open(auditEvent())
    await user.click(
      await screen.findByRole("button", { name: "Filter to actor Ada" })
    )
    expect(onQuery).toHaveBeenLastCalledWith(
      'actor_id:"11111111-1111-4111-8111-111111111111"'
    )
    await user.click(
      screen.getByRole("button", { name: "Filter to target Checkout" })
    )
    expect(onQuery).toHaveBeenLastCalledWith(
      'target:"dashboard/22222222-2222-4222-8222-222222222222"'
    )
  })

  it("steps to the previous and next event with the arrow keys", async () => {
    const { onSelect } = open(auditEvent())
    await screen.findByText("Checkout")
    const title = screen.getByRole("heading", { name: "dashboard.update" })
    fireEvent.keyDown(title, { key: "ArrowDown" })
    expect(onSelect).toHaveBeenLastCalledWith("e3")
    fireEvent.keyDown(title, { key: "ArrowUp" })
    expect(onSelect).toHaveBeenLastCalledWith("e1")
  })

  it("leaves the arrow keys alone while focus is elsewhere on the page", async () => {
    const { onSelect } = open(auditEvent())
    await screen.findByText("Checkout")
    const bar = document.createElement("button")
    document.body.append(bar)
    fireEvent.keyDown(bar, { key: "ArrowDown" })
    fireEvent.keyDown(document.body, { key: "ArrowUp" })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it("leaves the arrow keys to an input", async () => {
    const { onSelect } = open(auditEvent())
    await screen.findByText("Checkout")
    const input = document.createElement("input")
    document.body.append(input)
    fireEvent.keyDown(input, { key: "ArrowDown" })
    expect(onSelect).not.toHaveBeenCalled()
  })
})
