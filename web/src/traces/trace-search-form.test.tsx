import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { TraceSearchForm } from "./trace-search-form"

function form(value: Parameters<typeof TraceSearchForm>[0]["value"]) {
  const onSubmit = vi.fn()
  const view = renderWithProviders(
    <>
      <TraceSearchForm
        id="f"
        type="tempo"
        uid="tempo"
        value={value}
        onSubmit={onSubmit}
      />
      <button type="submit" form="f">
        Search
      </button>
    </>
  )
  return { ...view, onSubmit }
}

describe("TraceSearchForm", () => {
  it("lets TraceQL replace the fields while it has text", async () => {
    const { user, onSubmit } = form({
      service: "checkout",
      min: "1s",
      errors: "1",
      q: '{ name = "a" }',
    })
    expect(screen.getByLabelText("Service")).toBeDisabled()
    expect(screen.getByLabelText("Span name")).toBeDisabled()
    expect(screen.getByLabelText("Min span duration")).toBeDisabled()
    expect(screen.getByLabelText("Max span duration")).toBeDisabled()
    expect(screen.getByRole("checkbox")).toHaveAttribute(
      "aria-disabled",
      "true"
    )
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith({ q: '{ name = "a" }' })
  })

  it("brings the fields back once the TraceQL is cleared", async () => {
    const { user, onSubmit } = form({ service: "checkout", q: "{}" })
    await user.clear(screen.getByLabelText("TraceQL"))
    expect(screen.getByLabelText("Service")).toBeEnabled()
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ service: "checkout", q: undefined })
    )
  })
})

describe("TraceSearchForm on ClickHouse", () => {
  afterEach(() => vi.unstubAllGlobals())

  function clickhouseForm() {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ values: ["checkout", "payments"] }), {
            headers: { "Content-Type": "application/json" },
          })
      )
    )
    const onSubmit = vi.fn()
    const view = renderWithProviders(
      <>
        <TraceSearchForm
          id="f"
          type="clickhouse"
          uid="ch"
          value={{}}
          onSubmit={onSubmit}
        />
        <button type="submit" form="f">
          Search
        </button>
      </>
    )
    return { ...view, onSubmit }
  }

  it("lists the services in a combobox and takes one", async () => {
    const { user, onSubmit } = clickhouseForm()
    const field = screen.getByRole("combobox", { name: "Service" })
    await user.click(field)
    await user.click(await screen.findByRole("option", { name: "payments" }))
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ service: "payments" })
    )
  })

  it("keeps a service typed by hand", async () => {
    const { user, onSubmit } = clickhouseForm()
    await user.type(screen.getByRole("combobox", { name: "Service" }), "cart")
    await user.tab()
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ service: "cart" })
    )
  })
})

describe("TraceSearchForm on Jaeger", () => {
  afterEach(() => vi.unstubAllGlobals())

  function jaegerForm(value: Parameters<typeof TraceSearchForm>[0]["value"]) {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ values: ["checkout", "payments"] }), {
          headers: { "Content-Type": "application/json" },
        })
    )
    vi.stubGlobal("fetch", fetchMock)
    const onSubmit = vi.fn()
    const view = renderWithProviders(
      <>
        <TraceSearchForm
          id="f"
          type="jaeger"
          uid="jaeger"
          value={value}
          onSubmit={onSubmit}
        />
        <button type="submit" form="f">
          Search
        </button>
      </>
    )
    return { ...view, onSubmit, fetchMock }
  }

  it("lists the services under the service label and takes one", async () => {
    const { user, onSubmit, fetchMock } = jaegerForm({})
    await user.click(screen.getByRole("combobox", { name: "Service" }))
    await user.click(await screen.findByRole("option", { name: "payments" }))
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ service: "payments" })
    )
    const requested = fetchMock.mock.calls.map(([input]) =>
      input instanceof Request ? input.url : String(input)
    )
    expect(
      requested.some((url) => url.includes("/labels/service/values"))
    ).toBe(true)
  })

  it("names the written search Search and lets it replace the fields", async () => {
    const { user, onSubmit } = jaegerForm({ q: "http.status_code=500" })
    expect(screen.getByLabelText("Search")).toHaveValue("http.status_code=500")
    expect(screen.getByRole("combobox", { name: "Service" })).toBeDisabled()
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith({ q: "http.status_code=500" })
  })
})
