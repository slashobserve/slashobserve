import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"

import type { PlaceholderPanel as PlaceholderSpec } from "@/spec"

import PlaceholderPanel from "./placeholder-panel"

function render(extra: Partial<PlaceholderSpec>): string {
  return renderToStaticMarkup(
    createElement(PlaceholderPanel, {
      panel: {
        id: "ph",
        type: "placeholder",
        gridPos: { x: 0, y: 0, w: 6, h: 4 },
        reason: "Grafana panel type barchart is not supported yet.",
        ...extra,
      },
      width: 400,
      height: 200,
      vars: {},
    })
  )
}

describe("placeholder panel", () => {
  it("shows the reason", () => {
    expect(render({})).toContain(
      "Grafana panel type barchart is not supported yet."
    )
  })

  it("names the Grafana panel type when the importer recorded it", () => {
    expect(render({ sourceType: "barchart" })).toContain(
      "Grafana panel type: barchart"
    )
  })

  it("leaves the type line out without a source type", () => {
    expect(render({})).not.toContain("Grafana panel type:")
  })

  it("renders the reason as text, not markup", () => {
    const html = render({ reason: "<img src=x onerror=alert(1)>" })
    expect(html).not.toContain("<img")
    expect(html).toContain("&lt;img")
  })
})
