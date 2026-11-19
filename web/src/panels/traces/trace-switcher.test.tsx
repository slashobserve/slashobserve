import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import {
  literalDatasource,
  switcherLabel,
  TraceSwitcher,
} from "./trace-switcher"

function switcher(index: number, count: number, datasource = "tempo") {
  const onChange = vi.fn()
  const view = renderWithProviders(
    <TraceSwitcher
      index={index}
      count={count}
      traceId="abc"
      datasource={datasource}
      range={{ from: "now-1h", to: "now" }}
      onChange={onChange}
    />,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, onChange }
}

describe("switcherLabel", () => {
  it("counts from one", () => {
    expect(switcherLabel(0, 8066)).toBe("Trace 1 of 8,066")
  })
})

describe("literalDatasource", () => {
  it("drops an empty or variable datasource", () => {
    expect(literalDatasource("")).toBeNull()
    expect(literalDatasource("${ds}")).toBeNull()
    expect(literalDatasource("tempo")).toBe("tempo")
  })
})

describe("TraceSwitcher", () => {
  it("steps through the traces and stops at the ends", async () => {
    const { user, onChange } = switcher(0, 3)
    expect(await screen.findByText("Trace 1 of 3")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Previous trace" })
    ).toBeDisabled()
    await user.click(screen.getByRole("button", { name: "Next trace" }))
    expect(onChange).toHaveBeenCalledWith(1)
  })

  it("disables Next on the last trace", async () => {
    switcher(2, 3)
    expect(
      await screen.findByRole("button", { name: "Next trace" })
    ).toBeDisabled()
  })

  it("links to the trace and hides the stepper for one trace", async () => {
    switcher(0, 1)
    const link = await screen.findByRole("link", { name: "Open trace" })
    expect(screen.queryByRole("button", { name: "Next trace" })).toBeNull()
    expect(link.getAttribute("href")).toContain("/t/abc")
    expect(link.getAttribute("href")).toContain("ds=tempo")
  })

  it("omits the link when the datasource is a variable", async () => {
    switcher(0, 2, "${ds}")
    await screen.findByText("Trace 1 of 2")
    expect(screen.queryByRole("link", { name: "Open trace" })).toBeNull()
  })
})
