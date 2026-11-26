import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { DatasourceStatus } from "@/datasources/queries"
import { renderWithProviders } from "@/test/render"

import { TraceSqlEditor, TraceSqlResults } from "./trace-sql-view"

vi.mock("@/query-editor/code-editor", () => ({
  CodeEditor: ({
    value,
    onChange,
    ariaLabel,
  }: {
    value: string
    onChange: (value: string) => void
    ariaLabel: string
  }) => (
    <textarea
      aria-label={ariaLabel}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}))

const datasource: DatasourceStatus = {
  uid: "ch",
  type: "clickhouse",
  name: "ClickHouse",
  source: "file",
  healthy: true,
  checked_at: "2026-11-20T12:00:00Z",
  dashboard_count: 0,
}

const traceId = "0af7651916cd43dd8448eb211c80319c"

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubQuery(answer: () => Response) {
  const sent: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/datasources/ch/labels") {
        return json({ labels: ["TraceId", "ServiceName"] })
      }
      if (url.pathname === "/api/v1/datasources/ch/query") {
        sent.push(JSON.parse(await request.text()))
        return answer()
      }
      return json({ error: "not found", code: "not_found" }, 404)
    })
  )
  return sent
}

function page(fields: { name: string; values: unknown[]; type?: string }[]) {
  return json({
    frames: [
      {
        ref: "A",
        name: "",
        labels: {},
        fields: fields.map((field) => ({ type: "string", ...field })),
      },
    ],
    meta: {
      rows: fields[0]?.values.length ?? 0,
      page: {
        page: 1,
        page_size: 50,
        total: 1,
        total_relation: "eq",
        has_next: false,
        has_prev: false,
        plain: true,
      },
    },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("TraceSqlEditor", () => {
  it("runs the written SQL from the button and from Ctrl+Enter", async () => {
    stubQuery(() => page([]))
    const onRun = vi.fn()
    const { user } = renderWithProviders(
      <TraceSqlEditor
        datasource={datasource}
        initial="SELECT 1"
        onRun={onRun}
        editorRef={{ current: null }}
      />
    )
    const editor = screen.getByLabelText("SQL query")
    await user.clear(editor)
    expect(screen.getByRole("button", { name: "Run query" })).toBeDisabled()
    await user.type(editor, "SELECT 2")
    await user.click(screen.getByRole("button", { name: "Run query" }))
    expect(onRun).toHaveBeenLastCalledWith("SELECT 2")
    await user.type(editor, "{Control>}{Enter}{/Control}")
    expect(onRun).toHaveBeenCalledTimes(2)
    expect(
      screen.getByText("Ctrl Space completes keywords and columns.", {
        exact: false,
      })
    ).toBeTruthy()
  })
})

describe("TraceSqlResults", () => {
  it("shows the rows as a table and links each trace id to its trace", async () => {
    const sent = stubQuery(() =>
      page([
        { name: "TraceId", values: [traceId] },
        { name: "ServiceName", values: ["checkout"] },
      ])
    )
    renderWithProviders(
      <TraceSqlResults
        datasource={datasource}
        query="SELECT TraceId, ServiceName FROM otel_traces"
        search={{ from: "now-6h" }}
        onPage={vi.fn()}
        onPageSize={vi.fn()}
        live={false}
      />,
      { route: { path: "/", url: "/" } }
    )
    const link = await screen.findByRole("link", { name: traceId })
    expect(link.getAttribute("href")).toContain(`/t/${traceId}`)
    expect(screen.getByText("checkout")).toBeTruthy()
    expect(screen.getByText("Each trace id opens its trace.")).toBeTruthy()
    expect(
      screen.getByRole("button", { name: "Add to dashboard" })
    ).toBeTruthy()
    expect(
      screen.getByRole("button", { name: "Add to observation" })
    ).toBeTruthy()
    expect(sent[0]).toMatchObject({
      query: "SELECT TraceId, ServiceName FROM otel_traces",
      kind: "instant",
      from: "now-6h",
      to: "now",
      as_written: true,
    })
  })

  it("shows time and duration columns readably with the raw value on hover", async () => {
    const at = Date.UTC(2026, 10, 20, 12, 30, 15, 832)
    stubQuery(() =>
      page([
        { name: "TraceId", values: [traceId] },
        { name: "StartTime", type: "time", values: [at] },
        { name: "Duration", type: "number", values: [17_277_411] },
        { name: "Spans", type: "number", values: [2] },
      ])
    )
    renderWithProviders(
      <TraceSqlResults
        datasource={datasource}
        query="SELECT TraceId FROM otel_traces"
        search={{}}
        onPage={vi.fn()}
        onPageSize={vi.fn()}
        live={false}
      />,
      { route: { path: "/", url: "/" } }
    )
    const duration = await screen.findByText("17.28 ms")
    expect(duration.getAttribute("title")).toBe("17277411 ns")
    expect(screen.getByTitle(`${at} ms`)).toBeTruthy()
    expect(screen.queryByText(String(at))).toBeNull()
    expect(screen.getByText("2")).toBeTruthy()
  })

  it("says how to link rows when the result has no trace id", async () => {
    stubQuery(() => page([{ name: "ServiceName", values: ["checkout"] }]))
    renderWithProviders(
      <TraceSqlResults
        datasource={datasource}
        query="SELECT ServiceName FROM otel_traces"
        search={{}}
        onPage={vi.fn()}
        onPageSize={vi.fn()}
        live={false}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByText("Select TraceId to open each trace from its row.")
    ).toBeTruthy()
    expect(screen.queryByRole("link")).toBeNull()
  })

  it("shows the database message when the query fails", async () => {
    stubQuery(() =>
      json(
        {
          error:
            "Syntax error: failed at position 8 (FROM): expected expression",
          code: "invalid_query",
        },
        400
      )
    )
    renderWithProviders(
      <TraceSqlResults
        datasource={datasource}
        query="SELECT FROM otel_traces"
        search={{}}
        onPage={vi.fn()}
        onPageSize={vi.fn()}
        live={false}
      />,
      { route: { path: "/", url: "/" } }
    )
    await waitFor(() =>
      expect(screen.getByText(/failed at position 8 \(FROM\)/)).toBeTruthy()
    )
  })
})
