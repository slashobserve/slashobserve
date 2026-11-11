import { screen } from "@testing-library/react"
import { describe, expect, test, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { HealthItem, HealthProblem } from "./health-labels"
import { HealthRow } from "./health-row"

const noDataMessage = "query returned no data"

function noData(index: number): HealthProblem {
  return {
    kind: "no_data",
    panel_id: String(index),
    panel_title: `CPU ${String(index).padStart(2, "0")}`,
    variable: null,
    code: "query_empty",
    message: noDataMessage,
  }
}

const failing: HealthProblem = {
  kind: "query_failed",
  panel_id: "1",
  panel_title: "Orders",
  variable: null,
  code: "bad_data",
  message: "parse error",
}

function item(fields: Partial<HealthItem>): HealthItem {
  return {
    id: "d1",
    title: "Node Exporter Full",
    url: "/d/d1/node-exporter-full",
    folder: { id: "f1", title: "Imported" },
    can_edit: true,
    checked_version: 1,
    checked_at: "2026-11-06T10:00:00Z",
    last_viewed_at: null,
    skipped: null,
    problems: [failing],
    repair: null,
    ...fields,
  } as HealthItem
}

const repair = {
  request_id: "0a1b2c3d-0000-4000-8000-0000000000a1",
  version: null,
  error: null,
  updated_at: "2026-11-06T10:00:00Z",
}

async function renderRow(
  it: HealthItem,
  { agentAvailable = true, onRepair = vi.fn() } = {}
) {
  const view = renderWithProviders(
    <table>
      <tbody>
        <HealthRow
          item={it}
          agentAvailable={agentAvailable}
          pending={false}
          onRepair={onRepair}
        />
      </tbody>
    </table>,
    { route: { path: "/health", url: "/health" } }
  )
  await screen.findByRole("link", { name: it.title })
  return view
}

describe("HealthRow", () => {
  test("groups 13 panels without data into one line with the message once", async () => {
    const { user } = await renderRow(
      item({ problems: Array.from({ length: 13 }, (_, i) => noData(i + 1)) })
    )
    expect(screen.getByText("No data")).toBeTruthy()
    expect(screen.getByText("· 13 panels")).toBeTruthy()
    expect(screen.getAllByText("No data in the last 7 days.")).toHaveLength(1)
    expect(screen.queryByText(/query returned/)).toBeNull()
    const toggle = screen.getByRole("button", { name: "Show panels" })
    expect(toggle.getAttribute("aria-expanded")).toBe("false")
    const panels = document.getElementById(
      toggle.getAttribute("aria-controls") ?? ""
    )
    expect(panels?.hidden).toBe(true)

    await user.click(toggle)

    expect(
      screen
        .getByRole("button", { name: "Hide panels" })
        .getAttribute("aria-expanded")
    ).toBe("true")
    expect(panels?.hidden).toBe(false)
    expect(panels?.textContent).toContain("CPU 01")
    expect(panels?.querySelectorAll("li")).toHaveLength(13)
  })

  test("offers Open and no Repair to a user who cannot edit", async () => {
    await renderRow(item({ can_edit: false }))
    expect(screen.getByRole("link", { name: "Open" })).toBeTruthy()
    expect(screen.queryByRole("button", { name: /Repair/ })).toBeNull()
  })

  test("disables Repair with its reason when no agent is configured", async () => {
    const onRepair = vi.fn()
    const { user } = await renderRow(item({}), {
      agentAvailable: false,
      onRepair,
    })
    const button = screen.getByRole("button", { name: "Repair" })
    expect(button.getAttribute("aria-disabled")).toBe("true")
    expect(button.getAttribute("title")).toBe("No agent is configured")
    expect(screen.getByText("No agent is configured").id).toBe(
      button.getAttribute("aria-describedby")
    )
    await user.click(button)
    expect(onRepair).not.toHaveBeenCalled()
  })

  test("asks for a repair when the agent can fix the dashboard", async () => {
    const onRepair = vi.fn()
    const { user } = await renderRow(item({}), { onRepair })
    await user.click(screen.getByRole("button", { name: "Repair" }))
    expect(onRepair).toHaveBeenCalledOnce()
  })

  test("shows a running repair", async () => {
    await renderRow(
      item({ repair: { ...repair, state: "running", outcome: "running" } })
    )
    expect(screen.getByText("Repairing")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Repair" })).toBeNull()
  })

  test("links a saved repair to the versions compare until the next check", async () => {
    await renderRow(
      item({
        checked_version: 1,
        repair: {
          ...repair,
          state: "succeeded",
          outcome: "repaired",
          version: 2,
        },
      })
    )
    expect(screen.getByText("Repaired")).toBeTruthy()
    const link = screen.getByRole("link", { name: "See changes" })
    const href = new URL(link.getAttribute("href") ?? "", "http://x")
    expect(href.pathname).toBe("/d/d1/versions")
    expect(href.searchParams.get("from")).toBe("1")
    expect(href.searchParams.get("to")).toBe("2")
  })

  test("shows why a repair failed and offers it again", async () => {
    await renderRow(
      item({
        repair: {
          ...repair,
          state: "failed",
          outcome: "failed",
          error: "the model gave up",
        },
      })
    )
    expect(screen.getByText("Repair failed")).toBeTruthy()
    expect(screen.getByText("The model gave up.")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Repair" })).toBeTruthy()
  })
})
