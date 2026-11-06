import { useRef, useState } from "react"
import { screen, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import type { DatasourceStatus } from "@/datasources/queries"
import { createFieldResolver } from "@/panels/field-config"
import type { LogDetail, LogEntry } from "@/panels/logs/logs-model"
import type { LogSelection } from "@/panels/logs/logs-panel"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { DetailDrawer, documentGroups, entryJson } from "./detail-drawer"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

function datasource(
  type: string,
  uid: string,
  extra: Partial<DatasourceStatus> = {}
): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "2026-11-07T12:00:00Z",
    dashboard_count: 0,
    ...extra,
  } as DatasourceStatus
}

const traceId = "4bf92f3577b34da6a3ce929d0e0e4736"
const resolver = createFieldResolver({
  id: "logs",
  type: "logs",
  title: "Logs",
  datasource: "ds",
  gridPos: { x: 0, y: 0, w: 24, h: 10 },
  queries: [{ query: "" }],
})

function texts(pairs: [string, string][]): LogDetail[] {
  return pairs.map(([name, text]) => ({ name, text }))
}

function selection(
  details: LogDetail[],
  labels: Record<string, string> = {}
): LogSelection {
  const entry: LogEntry = {
    key: "k",
    time: Date.UTC(2026, 10, 6, 14, 19, 56, 641),
    level: { word: "Error", kind: "error" },
    message: "acquirer-b returned 503",
    labels,
    repeats: 1,
    details,
  }
  return { entry, streamLabels: labels, fields: resolver }
}

const lokiDetails = texts([
  ["service_name", "checkout"],
  ["pod", "checkout-1"],
  ["trace_id", traceId],
])

const searchDetails = texts([
  ["Time", "Nov 6, 14:19:56.641"],
  ["TimeNs", "1762438796641000000"],
  ["Body", "acquirer-b returned 503"],
  ["level", "error"],
  ["ID", "abc"],
  ["Index", "logs-2026.11.06"],
  ["service", "payments"],
  ["Fields.payments.provider", "acquirer-b"],
  ["Fields.http.status", "503"],
])

type SetupOptions = {
  ds: DatasourceStatus
  selection: LogSelection
  others?: DatasourceStatus[]
  tab?: "table" | "json" | "context"
  onClose?: () => void
  query?: string
  run?: (text: string) => void
}

function Harness({
  ds,
  selection: selected,
  tab: initialTab = "table",
  query,
  run = () => {},
}: SetupOptions) {
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState(initialTab)
  const trigger = useRef<HTMLElement | null>(null)
  return (
    <LogsWorkspaceContext
      value={
        {
          datasource: ds,
          search: { q: query },
          run,
        } as unknown as LogsWorkspace
      }
    >
      <button
        type="button"
        onClick={(event) => {
          trigger.current = event.currentTarget
          setOpen(true)
        }}
      >
        Row
      </button>
      {open ? (
        <DetailDrawer
          selection={selected}
          tab={tab}
          onTabChange={setTab}
          onClose={() => setOpen(false)}
          trigger={trigger}
          id="drawer"
        />
      ) : null}
    </LogsWorkspaceContext>
  )
}

async function open(options: SetupOptions) {
  const host = document.createElement("div")
  document.body.append(host)
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), {
    datasources: [options.ds, ...(options.others ?? [])],
  })
  queryClient.setQueryData(datasourceKeys.labels(options.ds.uid, {}), {
    labels: ["service_name", "pod"],
    truncated: false,
  })
  const rendered = renderWithProviders(
    <PanelHostContext value={host}>
      <Harness {...options} />
    </PanelHostContext>,
    { queryClient, route: { path: "/logs", url: "/logs" } }
  )
  await rendered.user.click(await screen.findByRole("button", { name: "Row" }))
  return { ...rendered, host }
}

beforeEach(() => {
  vi.stubGlobal("fetch", () => new Promise(() => {}))
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("documentGroups", () => {
  it("splits core fields from the rest, hides Body and TimeNs, strips the Fields prefix and names the index", () => {
    const { core, other } = documentGroups(searchDetails)
    expect(core.map((detail) => detail.name)).toEqual([
      "Time",
      "level",
      "service",
      "ID",
    ])
    expect(other.map((detail) => detail.name)).toEqual([
      "_index",
      "payments.provider",
      "http.status",
    ])
  })

  it("orders core fields as time, level, service, trace id, span id, id", () => {
    const { core } = documentGroups(
      texts([
        ["ID", "1"],
        ["span_id", "2"],
        ["trace_id", "3"],
        ["service", "4"],
        ["level", "5"],
        ["Time", "6"],
      ])
    )
    expect(core.map((detail) => detail.name)).toEqual([
      "Time",
      "level",
      "service",
      "trace_id",
      "span_id",
      "ID",
    ])
  })
})

describe("entryJson", () => {
  it("holds the time, level, line, labels and every detail by name", () => {
    const { entry, fields } = selection(searchDetails, { index: "logs" })
    expect(entryJson(entry, fields)).toEqual({
      time: "2026-11-06T14:19:56.641Z",
      level: "Error",
      message: "acquirer-b returned 503",
      labels: { index: "logs" },
      fields: Object.fromEntries(
        searchDetails.map((detail) => [
          detail.name,
          "text" in detail ? detail.text : "",
        ])
      ),
    })
  })
})

describe("DetailDrawer", () => {
  it("lists one term per detail on the Table tab", async () => {
    await open({
      ds: datasource("loki", "loki-1"),
      selection: selection(lokiDetails, { service_name: "checkout" }),
    })
    const drawer = await screen.findByRole("complementary", { name: "Line" })
    expect(within(drawer).getAllByRole("term")).toHaveLength(lokiDetails.length)
    expect(within(drawer).getByText("acquirer-b returned 503")).toBeTruthy()
  })

  describe("filtering from a line", () => {
    const lokiLine = () =>
      selection(
        texts([
          ["service_name", "checkout"],
          ["trace_id", "abc"],
          ["status", "500"],
        ]),
        { service_name: "checkout", trace_id: "abc" }
      )

    it("groups a Loki line into labels, parsed fields and structured metadata with two filter buttons per field", async () => {
      await open({
        ds: datasource("loki", "loki-1"),
        selection: lokiLine(),
        query: '{service_name="checkout"}',
      })
      const drawer = await screen.findByRole("complementary", { name: "Line" })
      for (const name of ["Labels", "Parsed fields", "Structured metadata"]) {
        expect(within(drawer).getByRole("region", { name })).toBeTruthy()
      }
      expect(
        within(drawer).getAllByRole("button", { name: /^(Include|Exclude) / })
      ).toHaveLength(6)
    })

    it.each([
      {
        label: "a parsed field into a pipeline stage",
        button: "Include status 500",
        expected: '{service_name="checkout"} | status="500"',
      },
      {
        label: "an excluded parsed field into a negative stage",
        button: "Exclude status 500",
        expected: '{service_name="checkout"} | status!="500"',
      },
      {
        label: "a per-line label into a pipeline stage, never the selector",
        button: "Include trace_id abc",
        expected: '{service_name="checkout"} | trace_id="abc"',
      },
    ])("writes $label through run", async ({ button, expected }) => {
      const run = vi.fn()
      const { user } = await open({
        ds: datasource("loki", "loki-1"),
        selection: lokiLine(),
        query: '{service_name="checkout"}',
        run,
      })
      await user.click(await screen.findByRole("button", { name: button }))
      expect(run).toHaveBeenCalledExactlyOnceWith(expected)
    })

    it("disables a button whose filter is already in the query", async () => {
      const run = vi.fn()
      const { user } = await open({
        ds: datasource("loki", "loki-1"),
        selection: lokiLine(),
        query: '{service_name="checkout"} | status="500"',
        run,
      })
      const button = await screen.findByRole("button", {
        name: "Include status 500",
      })
      expect(button.getAttribute("aria-disabled")).toBe("true")
      await user.click(button)
      expect(run).not.toHaveBeenCalled()
    })

    it("lists ClickHouse columns apart from attributes and writes into the WHERE clause", async () => {
      const run = vi.fn()
      const { user } = await open({
        ds: datasource("clickhouse", "ch-1"),
        selection: selection(
          texts([
            ["ServiceName", "checkout"],
            ["http.status", "500"],
          ])
        ),
        query: "SELECT * FROM otel_logs WHERE 1 LIMIT 5",
        run,
      })
      const drawer = await screen.findByRole("complementary", { name: "Line" })
      expect(
        within(drawer).getByRole("region", { name: "Columns" })
      ).toBeTruthy()
      expect(
        within(drawer).getByRole("region", { name: "Attributes" })
      ).toBeTruthy()
      await user.click(
        within(drawer).getByRole("button", { name: "Include http.status 500" })
      )
      expect(run).toHaveBeenCalledExactlyOnceWith(
        "SELECT * FROM otel_logs WHERE 1 AND LogAttributes['http.status'] = '500' LIMIT 5"
      )
    })

    it("appends KQL on a search document, offers exists and leaves core fields alone", async () => {
      const run = vi.fn()
      const { user } = await open({
        ds: datasource("elasticsearch", "es-1", { index: "logs-*" }),
        selection: selection(searchDetails),
        query: "level:error",
        run,
      })
      const drawer = await screen.findByRole("complementary", {
        name: "Document",
      })
      const core = within(drawer).getByRole("region", { name: "Core fields" })
      expect(within(core).queryAllByRole("button")).toHaveLength(0)
      await user.click(
        within(drawer).getByRole("button", { name: "Include http.status 503" })
      )
      await user.click(
        within(drawer).getByRole("button", { name: "Require http.status" })
      )
      expect(run.mock.calls).toEqual([
        ['level:error and http.status:"503"'],
        ["level:error and http.status:*"],
      ])
    })

    it("disables the buttons on a Lucene query", async () => {
      await open({
        ds: datasource("elasticsearch", "es-1", { index: "logs-*" }),
        selection: selection(searchDetails),
        query: "lucene: level:error",
      })
      const button = await screen.findByRole("button", {
        name: "Include http.status 503",
      })
      expect(button.getAttribute("aria-disabled")).toBe("true")
    })
  })

  it("shows the selector on the Context tab", async () => {
    const { user } = await open({
      ds: datasource("loki", "loki-1"),
      selection: selection(lokiDetails, { service_name: "checkout" }),
    })
    await user.click(await screen.findByRole("tab", { name: "Context" }))
    expect(await screen.findByText('{service_name="checkout"}')).toBeTruthy()
  })

  it("opens the Context tab from Show context", async () => {
    const { user } = await open({
      ds: datasource("loki", "loki-1"),
      selection: selection(lokiDetails, { service_name: "checkout" }),
    })
    await user.click(
      await screen.findByRole("button", { name: "Show context" })
    )
    expect(await screen.findByText('{service_name="checkout"}')).toBeTruthy()
  })

  it("prints the whole entry on the JSON tab", async () => {
    const { user } = await open({
      ds: datasource("loki", "loki-1"),
      selection: selection(lokiDetails, { service_name: "checkout" }),
    })
    await user.click(await screen.findByRole("tab", { name: "JSON" }))
    const printed = (await screen.findByText(/"message"/)).textContent ?? ""
    expect(JSON.parse(printed)).toMatchObject({
      level: "Error",
      message: "acquirer-b returned 503",
      fields: { pod: "checkout-1" },
    })
  })

  it.each([
    { label: "a trace id and a trace datasource", trace: true, ids: true },
    { label: "a trace id but no trace datasource", trace: false, ids: true },
    { label: "a trace datasource but no trace id", trace: true, ids: false },
  ])("offers Open trace only with $label", async ({ trace, ids }) => {
    await open({
      ds: datasource("loki", "loki-1"),
      others: trace ? [datasource("tempo", "tempo-1")] : [],
      selection: selection(ids ? lokiDetails : lokiDetails.slice(0, 2), {
        service_name: "checkout",
      }),
    })
    await screen.findByRole("complementary", { name: "Line" })
    const link = screen.queryByRole("link", { name: /Open trace/ })
    if (trace && ids) {
      expect(link?.textContent).toBe("Open trace 4bf92f35")
    } else {
      expect(link).toBeNull()
    }
  })

  it("groups a search document into core and other fields with counts", async () => {
    await open({
      ds: datasource("elasticsearch", "es-1", { index: "logs-*" }),
      selection: selection(searchDetails),
    })
    const drawer = await screen.findByRole("complementary", {
      name: "Document",
    })
    const core = within(drawer).getByRole("region", { name: "Core fields" })
    const other = within(drawer).getByRole("region", { name: "Other fields" })
    expect(within(core).getAllByRole("term")).toHaveLength(4)
    expect(within(other).getAllByRole("term")).toHaveLength(3)
    expect(within(other).getByRole("heading").textContent).toBe(
      "Other fieldslogs-*, 3"
    )
    expect(
      within(drawer)
        .getByRole("button", { name: "Surrounding documents" })
        .hasAttribute("disabled")
    ).toBe(true)
    expect(
      within(drawer).getByText(/^logs-2026\.11\.06, .+:\d\d\.641, Error$/)
    ).toBeTruthy()
  })

  it("closes on Escape and returns focus to the row that opened it", async () => {
    const { user } = await open({
      ds: datasource("loki", "loki-1"),
      selection: selection(lokiDetails, { service_name: "checkout" }),
    })
    await screen.findByRole("complementary", { name: "Line" })
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("complementary", { name: "Line" })).toBeNull()
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Row" })
    )
  })
})
