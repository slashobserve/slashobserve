import { act, fireEvent, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys, observeKeys } from "@/api/keys"
import type { InputMode } from "@/ask/input-mode"
import type { DatasourceStatus } from "@/datasources/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { decodePills } from "./pills"
import { QueryBar } from "./query-bar"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

function status(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    index: type === "elasticsearch" ? "logs-*" : undefined,
    healthy: true,
    checked_at: "2026-11-07T12:00:00Z",
    dashboard_count: 0,
  }
}

function setup({
  type = "elasticsearch",
  draft = "level:error",
  mode = "code",
  agentAvailable = true,
  labelsOpen = false,
  runFailed = false,
}: {
  type?: string
  draft?: string
  mode?: InputMode
  agentAvailable?: boolean
  labelsOpen?: boolean
  runFailed?: boolean
} = {}) {
  const run = vi.fn()
  const setMode = vi.fn()
  const setDraft = vi.fn()
  const focusQuery = vi.fn()
  const setLabelsOpen = vi.fn()
  const update = vi.fn()
  const workspace = {
    datasource: status("ds", type),
    datasources: [],
    search: {},
    update,
    draft,
    setDraft,
    run,
    range: { from: "now-1h", to: "now" },
    results: { isError: runFailed },
    mode,
    setMode,
    announce: vi.fn(),
    focusQuery,
    registerQueryFocus: vi.fn(),
    labels: { open: labelsOpen, setOpen: setLabelsOpen },
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), { available: agentAvailable })
  queryClient.setQueryData(datasourceKeys.labels("ds", {}), {
    labels: ["service_name", "level"],
    truncated: false,
  })
  queryClient.setQueryData(datasourceKeys.labels("ds", { search: undefined }), {
    labels: ["service_name", "level"],
    truncated: false,
  })
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <QueryBar.ModeSwitch />
      <QueryBar />
    </LogsWorkspaceContext>,
    { queryClient, route: { path: "/", url: "/" } }
  )
  return { ...view, run, setMode, setDraft, focusQuery, setLabelsOpen, update }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

const brokenQuery = "log.level : (error or warn and message : payment"

describe("QueryBar", () => {
  it.each([
    ["elasticsearch", "KQL"],
    ["opensearch", "KQL"],
    ["loki", "LogQL"],
    ["clickhouse", "SQL"],
  ])("names the code mode of %s %s", async (type, label) => {
    setup({ type })
    expect(
      await screen.findByRole("radiogroup", { name: "Query mode" })
    ).toBeInTheDocument()
    expect(screen.getByRole("radio", { name: label })).toBeChecked()
  })

  it.each([
    ["elasticsearch", ["Builder", "KQL", "Ask"]],
    ["opensearch", ["Builder", "KQL", "Ask"]],
    ["loki", ["Builder", "LogQL", "Ask"]],
    ["clickhouse", ["SQL", "Ask"]],
  ])("offers the segments of %s", async (type, words) => {
    setup({ type })
    await screen.findByRole("radiogroup", { name: "Query mode" })
    expect(
      screen.getAllByRole("radio").map((radio) => radio.textContent)
    ).toEqual(words)
  })

  it("runs the query from the Run query button", async () => {
    const { run, user } = setup()
    await user.click(await screen.findByRole("button", { name: "Run query" }))
    expect(run).toHaveBeenCalledOnce()
  })

  it("does not run an empty query", async () => {
    const { run, user } = setup({ draft: "  " })
    const button = await screen.findByRole("button", { name: "Run query" })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(run).not.toHaveBeenCalled()
  })

  it("moves from code to Ask on Ctrl+I and focuses the query field", async () => {
    const { setMode, focusQuery, user } = setup()
    await screen.findByRole("radiogroup", { name: "Query mode" })
    await act(async () => {
      await user.keyboard("{Control>}i{/Control}")
    })
    expect(setMode).toHaveBeenCalledWith("ask", true)
    expect(focusQuery).toHaveBeenCalled()
  })

  it("moves from Ask to the builder on Ctrl+I when it can show the query", async () => {
    const { setMode, user } = setup({ mode: "ask" })
    await screen.findByRole("radiogroup", { name: "Query mode" })
    await act(async () => {
      await user.keyboard("{Control>}i{/Control}")
    })
    expect(setMode).toHaveBeenCalledWith("builder", true)
  })

  it("moves from Ask to code on Ctrl+I when the builder cannot show the query", async () => {
    const { setMode, user } = setup({ mode: "ask", draft: "a:1 or b:2" })
    await screen.findByRole("radiogroup", { name: "Query mode" })
    await act(async () => {
      await user.keyboard("{Control>}i{/Control}")
    })
    expect(setMode).toHaveBeenCalledWith("code", true)
  })

  it("keeps Ask unavailable without a model", async () => {
    const { setMode, user } = setup({ agentAvailable: false })
    const ask = await screen.findByRole("radio", { name: "Ask" })
    expect(ask).toHaveAttribute("aria-disabled", "true")
    await user.click(ask)
    expect(setMode).not.toHaveBeenCalled()
  })

  it("greys the builder and says why when the query has an or at the top", async () => {
    const { setMode, user } = setup({ draft: "a:1 or b:2" })
    const builder = await screen.findByRole("radio", { name: "Builder" })
    expect(builder).toHaveAttribute("aria-disabled", "true")
    await user.click(builder)
    expect(setMode).not.toHaveBeenCalled()
    expect(builder).toHaveAccessibleDescription(
      "The builder cannot show this query. Edit it as KQL."
    )
  })

  it("switches to the builder from the segment", async () => {
    const { setMode, user } = setup()
    await user.click(await screen.findByRole("radio", { name: "Builder" }))
    expect(setMode).toHaveBeenCalledWith("builder", true)
  })

  it("shows the filter chips instead of the editor in builder mode", async () => {
    setup({ mode: "builder", draft: "log.level:error and message:payment" })
    expect(
      await screen.findByRole("button", {
        name: "Edit filter log.level : error",
      })
    ).toBeVisible()
    expect(screen.getByRole("radio", { name: "Builder" })).toBeChecked()
    expect(screen.queryByLabelText("KQL query")).toBeNull()
  })

  it("falls back to the code editor when builder mode cannot show the query", async () => {
    setup({ mode: "builder", draft: "a:1 or b:2" })
    expect(await screen.findByRole("radio", { name: "KQL" })).toBeChecked()
    expect(screen.getByLabelText("KQL query")).toBeInTheDocument()
  })

  it("opens the label browser from the Builder segment on Loki", async () => {
    const { setLabelsOpen, user } = setup({ type: "loki", draft: "" })
    await user.click(await screen.findByRole("radio", { name: "Builder" }))
    expect(setLabelsOpen).toHaveBeenCalledWith(true)
  })

  it("shows the label browser and no Labels button on Loki with Builder on", async () => {
    setup({ type: "loki", labelsOpen: true })
    expect(
      await screen.findByRole("region", { name: "Label browser" })
    ).toBeInTheDocument()
    expect(screen.getByRole("radio", { name: "Builder" })).toBeChecked()
    expect(screen.queryByRole("button", { name: "Labels" })).toBeNull()
  })

  it("has no label browser on ClickHouse", async () => {
    setup({ type: "clickhouse" })
    await screen.findByRole("radiogroup", { name: "Query mode" })
    expect(screen.queryByRole("region", { name: "Label browser" })).toBeNull()
  })

  it("labels the editor and the help line by language", async () => {
    setup()
    expect(await screen.findByLabelText("KQL query")).toBeInTheDocument()
    expect(
      screen.getByText(
        /Ctrl Space completes field names from the logs-\* mapping\./
      )
    ).toBeVisible()
    expect(screen.getByRole("link", { name: "KQL reference" })).toBeVisible()
  })

  it("shows the caret and the fix for the board's broken query without running it", async () => {
    const { run, setDraft, user } = setup({ draft: brokenQuery })
    await user.click(await screen.findByRole("button", { name: "Run query" }))
    expect(run).not.toHaveBeenCalled()
    expect(
      screen.getByText(
        `${" ".repeat(brokenQuery.indexOf(":", 30))}^ expected ) here`,
        {
          normalizer: (text) => text,
        }
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        "Nothing ran. Checked by the KQL parser before Elasticsearch is asked"
      )
    ).toBeVisible()
    expect(
      screen.getByText("This query could not be read, so it did not run")
    ).toBeVisible()
    await user.click(
      screen.getByRole("button", {
        name: "Apply the suggested fix: log.level : (error or warn) and message : payment",
      })
    )
    const fixed = "log.level : (error or warn) and message : payment"
    expect(setDraft).toHaveBeenCalledWith(fixed)
    expect(run).toHaveBeenCalledWith(fixed)
  })

  it("opens the fixed query in the builder from the card", async () => {
    const { setDraft, setMode, user } = setup({ draft: "a:(b" })
    await user.click(await screen.findByRole("button", { name: "Run query" }))
    await user.click(
      screen.getByRole("button", { name: "Open in the builder" })
    )
    expect(setDraft).toHaveBeenCalledWith("a:(b)")
    expect(setMode).toHaveBeenCalledWith("builder")
  })

  it("drops the card as soon as the text changes", async () => {
    const { rerender } = setup({ draft: "a:(b" })
    expect(rerender).toBeDefined()
    expect(
      screen.queryByText("This query could not be read, so it did not run")
    ).toBeNull()
  })

  it("prefixes lucene: and runs from Switch to Lucene", async () => {
    const { run, setDraft, user } = setup({ draft: "+level:error -message:(a" })
    await user.click(await screen.findByRole("button", { name: "Run query" }))
    expect(run).not.toHaveBeenCalled()
    await user.click(
      screen.getAllByRole("button", { name: "Switch to Lucene" })[0]
    )
    const text = "lucene: +level:error -message:(a"
    expect(setDraft).toHaveBeenCalledWith(text)
    expect(run).toHaveBeenCalledWith(text)
  })

  it.each(["lucene: (a", '{"query": {'])(
    "leaves %s to the server's lint",
    async (draft) => {
      const { run, user } = setup({ draft })
      await user.click(await screen.findByRole("button", { name: "Run query" }))
      expect(run).toHaveBeenCalledOnce()
    }
  )

  it("rewrites the prefix when the Language select changes", async () => {
    const { setDraft, user } = setup({ draft: "level:error" })
    await user.click(await screen.findByRole("combobox", { name: "Language" }))
    await user.click(await screen.findByRole("option", { name: "Lucene" }))
    expect(setDraft).toHaveBeenCalledWith("lucene: level:error")
  })

  it("shows the current form in the Language select", async () => {
    setup({ draft: "lucene: level:error" })
    expect(
      await screen.findByRole("combobox", { name: "Language" })
    ).toHaveTextContent("Lucene")
  })

  it("has no Language select on Loki", async () => {
    setup({ type: "loki" })
    await screen.findByRole("radiogroup", { name: "Query mode" })
    expect(screen.queryByRole("combobox", { name: "Language" })).toBeNull()
  })

  it("keeps the last good run and offers the datasource check after a failed run", async () => {
    setup({ runFailed: true })
    expect(await screen.findByText(/^No results\./)).toBeVisible()
    expect(
      await screen.findByRole("link", { name: "Check the datasource" })
    ).toHaveAttribute("href", "/datasources")
  })

  it("offers to remove the clause of an unknown field only when the builder can show it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          diagnostics: [
            {
              severity: "warning",
              message: 'unknown field "payments.provider"',
              start: 0,
              end: 17,
            },
          ],
        })
      )
    )
    const { user, setDraft } = setup({ draft: "payments.provider:x and a:1" })
    expect(
      await screen.findByText(
        "payments.provider is not in the logs-* mapping",
        {},
        { timeout: 3000 }
      )
    ).toBeVisible()
    await user.click(screen.getByRole("button", { name: "Remove this clause" }))
    expect(setDraft).toHaveBeenCalledWith("a : 1")
  })

  it("offers no removal when the clause sits inside an or", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          diagnostics: [
            {
              severity: "warning",
              message: 'unknown field "payments.provider"',
              start: 0,
              end: 17,
            },
          ],
        })
      )
    )
    setup({ draft: "payments.provider:x or a:1" })
    expect(
      await screen.findByText(
        "payments.provider is not in the logs-* mapping",
        {},
        { timeout: 3000 }
      )
    ).toBeVisible()
    expect(
      screen.queryByRole("button", { name: "Remove this clause" })
    ).toBeNull()
  })

  describe("pasting a Discover link", () => {
    const link =
      "https://kibana.example.com/app/discover#/?" +
      new URLSearchParams({
        _g: "(time:(from:now-15m,to:now))",
        _a: `(columns:!(message,log.level),filters:!((meta:(key:service.name,negate:!f,params:(query:checkout),type:phrase)),(meta:(key:bytes,negate:!f,params:(gte:1),type:range))),query:(language:kuery,query:'level:error'))`,
      }).toString()

    async function paste(text: string, label = "KQL query") {
      const editor = await screen.findByLabelText(label)
      return fireEvent.paste(editor, {
        clipboardData: { getData: () => text },
      })
    }

    it("fills the query, range, columns and filters and says what it did", async () => {
      const { run, setDraft, update } = setup()
      expect(await paste(link)).toBe(false)
      expect(setDraft).toHaveBeenCalledWith("level:error")
      expect(run).toHaveBeenCalledWith("level:error")
      const written = update.mock.calls[0][0] as Record<string, string>
      expect(written).toMatchObject({
        q: "level:error",
        from: "now-15m",
        to: "now",
        cols: "message,log.level",
      })
      expect(decodePills(written.f)).toMatchObject([
        { kind: "is", field: "service.name", value: "checkout" },
      ])
      expect(
        await screen.findByText(
          "Filled from a Discover link: query, time range, 2 columns, 1 filter. 1 filter from the URL was not applied."
        )
      ).toBeVisible()
    })

    it("hides the note when dismissed", async () => {
      const { user } = setup()
      await paste(link)
      await user.click(await screen.findByRole("button", { name: "Dismiss" }))
      expect(screen.queryByText(/Filled from a Discover link/)).toBeNull()
    })

    it("leaves the draft alone when the link cannot be read", async () => {
      const { run, setDraft, update } = setup()
      expect(
        await paste("https://kibana.example.com/app/discover#/?_a=(query:('x)")
      ).toBe(false)
      expect(
        await screen.findByText("That Discover link could not be read.")
      ).toBeVisible()
      expect(setDraft).not.toHaveBeenCalled()
      expect(update).not.toHaveBeenCalled()
      expect(run).not.toHaveBeenCalled()
    })

    it("leaves other pasted text to the editor", async () => {
      const { setDraft, update } = setup()
      await paste("level:error")
      expect(setDraft).toHaveBeenCalledWith("level:errorlevel:error")
      expect(update).not.toHaveBeenCalled()
    })

    it("pastes the link as plain text on Loki", async () => {
      const { setDraft, update } = setup({ type: "loki", draft: "" })
      await paste(link, "LogQL query")
      expect(setDraft).toHaveBeenCalledWith(link)
      expect(update).not.toHaveBeenCalled()
    })
  })
})
