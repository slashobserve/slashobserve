import { screen } from "@testing-library/react"
import { describe, expect, test } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { HealthItem } from "./health-labels"
import { RepairControl } from "./repair-control"

function item(fields: Partial<HealthItem>): HealthItem {
  return {
    id: "d1",
    title: "Checkout",
    url: "/d/d1/checkout",
    folder: { id: "f1", title: "General" },
    can_edit: true,
    checked_version: 1,
    checked_at: "2026-11-06T10:00:00Z",
    last_viewed_at: null,
    skipped: null,
    problems: [
      {
        kind: "query_failed",
        panel_id: "1",
        panel_title: "Orders",
        variable: null,
        code: "bad_data",
        message: "parse error",
      },
    ],
    repair: null,
    ...fields,
  } as HealthItem
}

function renderControl(
  it: HealthItem,
  props: Partial<Parameters<typeof RepairControl>[0]> = {}
) {
  return renderWithProviders(
    <div data-testid="control">
      <RepairControl
        item={it}
        agentAvailable
        pending={false}
        onRepair={() => {}}
        {...props}
      />
    </div>,
    { route: { path: "/", url: "/" } }
  )
}

describe("RepairControl", () => {
  test("disables Repair with the blocked reason as its description", async () => {
    renderControl(item({}), { blockedReason: "Finish the layout first" })
    const button = await screen.findByRole("button", { name: "Repair" })
    expect(button.getAttribute("aria-disabled")).toBe("true")
    expect(button.getAttribute("title")).toBe("Finish the layout first")
    expect(screen.getByText("Finish the layout first").id).toBe(
      button.getAttribute("aria-describedby")
    )
  })

  test("names the missing agent before the blocked reason", async () => {
    renderControl(item({}), {
      agentAvailable: false,
      blockedReason: "Finish the layout first",
    })
    const button = await screen.findByRole("button", { name: "Repair" })
    expect(button.getAttribute("title")).toBe("No agent is configured")
  })

  test("renders nothing for a dashboard that cannot be repaired unless showOpen", async () => {
    const viewer = item({ can_edit: false })
    const hidden = renderControl(viewer)
    expect((await screen.findByTestId("control")).textContent).toBe("")
    expect(screen.queryByRole("link", { name: "Open" })).toBeNull()
    hidden.unmount()

    renderControl(viewer, { showOpen: true })
    expect(await screen.findByRole("link", { name: "Open" })).toBeTruthy()
  })
})
