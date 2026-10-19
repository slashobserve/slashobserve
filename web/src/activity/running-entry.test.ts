import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"

import { RunningLinkContent } from "./running-entry"

describe("RunningLinkContent", () => {
  const markup = renderToStaticMarkup(
    createElement(RunningLinkContent, { stepText: "Running queries, step 3" })
  )

  it("keeps the current step in the link's accessible text", () => {
    expect(markup).toContain("Running queries, step 3")
    expect(markup).not.toContain("aria-label")
  })

  it("adds the purpose and the new tab hint as screen reader text", () => {
    expect(markup).toContain(
      '<span class="sr-only"> of the running agent request, opens in a new tab</span>'
    )
  })
})
