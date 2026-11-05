import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { AnnotationLayer } from "./annotation-layer"
import type { Annotation } from "./queries"

const t0 = Date.parse("2026-11-01T10:00:00Z")
const minute = 60_000

function annotation(over: Partial<Annotation> = {}): Annotation {
  return {
    id: "a",
    dashboard_id: null,
    panel_id: null,
    time_from: new Date(t0 + 10 * minute).toISOString(),
    time_to: null,
    text: "Deploy checkout v1.4.2",
    tags: ["deploy", "checkout"],
    created_by: null,
    created_by_name: "ci-deploy",
    created_at: new Date(t0).toISOString(),
    ...over,
  }
}

function layer(annotations: Annotation[]) {
  return render(
    <AnnotationLayer
      annotations={annotations}
      fromMs={t0}
      toMs={t0 + 100 * minute}
      left={40}
      top={10}
      width={1000}
      height={200}
      zone="UTC"
    />
  )
}

describe("AnnotationLayer", () => {
  it("draws one labelled flag per cluster", () => {
    layer([
      annotation({ id: "a" }),
      annotation({
        id: "b",
        text: "Rollback",
        time_from: new Date(t0 + 10 * minute + 1000).toISOString(),
      }),
      annotation({
        id: "c",
        text: "Incident",
        time_from: new Date(t0 + 80 * minute).toISOString(),
      }),
    ])
    const flags = screen.getAllByRole("button")
    expect(flags).toHaveLength(2)
    expect(flags[0]).toHaveAccessibleName(
      "Annotation: Rollback, 2026-11-01 10:10:01 and 1 more"
    )
    expect(flags[1]).toHaveAccessibleName(
      "Annotation: Incident, 2026-11-01 11:20:00"
    )
  })

  it("shows the text, tags, author and time on hover", async () => {
    const user = userEvent.setup()
    layer([annotation()])
    await user.hover(screen.getByRole("button"))
    const card = screen.getByRole("group", { name: "Annotation" })
    expect(card).toHaveTextContent("Deploy checkout v1.4.2")
    expect(card).toHaveTextContent("deploy")
    expect(card).toHaveTextContent("checkout")
    expect(card).toHaveTextContent("ci-deploy")
    expect(card).toHaveTextContent("2026-11-01 10:10:00")
    expect(card).toHaveTextContent("Every dashboard")
    expect(screen.getByRole("button")).toHaveAttribute("aria-expanded", "true")
  })

  it("renders a same-origin path in the text as a link", async () => {
    const user = userEvent.setup()
    layer([annotation({ text: "Pinned /findings/abc", created_by_name: null })])
    await user.hover(screen.getByRole("button"))
    expect(screen.getByRole("link", { name: "/findings/abc" })).toHaveAttribute(
      "href",
      "/findings/abc"
    )
    expect(screen.getByRole("group")).toHaveTextContent("Unknown")
  })

  it("opens on focus and closes on Escape with focus back on the flag", async () => {
    const user = userEvent.setup()
    layer([annotation()])
    await user.tab()
    const flag = screen.getByRole("button")
    expect(flag).toHaveFocus()
    expect(screen.getByRole("group")).toBeInTheDocument()
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("group")).not.toBeInTheDocument()
    expect(flag).toHaveFocus()
  })

  it("lists at most five annotations of a cluster and counts the rest", async () => {
    const user = userEvent.setup()
    layer(
      Array.from({ length: 7 }, (_, i) =>
        annotation({
          id: `a${i}`,
          text: `note ${i}`,
          time_from: new Date(t0 + 10 * minute + i * 1000).toISOString(),
        })
      )
    )
    await user.hover(screen.getByRole("button"))
    const card = screen.getByRole("group")
    expect(card).toHaveTextContent("note 6")
    expect(card).not.toHaveTextContent("note 1")
    expect(card).toHaveTextContent("2 more")
  })

  it("stops a press on the flag from reaching the chart", () => {
    let reached = false
    render(
      <div onMouseDown={() => (reached = true)}>
        <AnnotationLayer
          annotations={[annotation()]}
          fromMs={t0}
          toMs={t0 + 100 * minute}
          left={0}
          top={0}
          width={1000}
          height={200}
          zone="UTC"
        />
      </div>
    )
    fireEvent.mouseDown(screen.getByRole("button"))
    expect(reached).toBe(false)
  })

  it("uses no animation class in the card", async () => {
    const user = userEvent.setup()
    const { container } = layer([annotation()])
    await user.hover(screen.getByRole("button"))
    for (const element of container.querySelectorAll("*")) {
      expect(element.getAttribute("class") ?? "").not.toMatch(/animate/)
    }
  })
})
