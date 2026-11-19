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

  function clickhouseForm(
    value: Parameters<typeof TraceSearchForm>[0]["value"] = {}
  ) {
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

  it("lets written SQL replace the fields while it has text", async () => {
    const { user, onSubmit } = clickhouseForm({
      service: "checkout",
      q: "SELECT TraceId FROM otel_traces",
    })
    expect(screen.getByRole("combobox", { name: "Service" })).toBeDisabled()
    expect(screen.getByLabelText("Span name")).toBeDisabled()
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith({
      q: "SELECT TraceId FROM otel_traces",
    })
  })

  it("searches with the fields when the SQL is empty", async () => {
    const { user, onSubmit } = clickhouseForm()
    await user.type(screen.getByLabelText("Span name"), "POST /pay")
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ span: "POST /pay", q: undefined })
    )
  })

  it("writes the fields as SQL to start from", async () => {
    const { user, onSubmit } = clickhouseForm({ span: "POST /pay" })
    await user.click(screen.getByRole("button", { name: "Write as SQL" }))
    const sql = screen.getByLabelText<HTMLTextAreaElement>("SQL")
    expect(sql.value).toContain("FROM otel_traces")
    expect(sql.value).toContain("SpanName = 'POST /pay'")
    expect(screen.getByLabelText("Span name")).toBeDisabled()
    await user.click(screen.getByRole("button", { name: "Search" }))
    expect(onSubmit).toHaveBeenCalledWith({ q: sql.value })
  })

  it("has no SQL field on Tempo", () => {
    form({})
    expect(screen.queryByLabelText("SQL")).toBeNull()
  })
})
