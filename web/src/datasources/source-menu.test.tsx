import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { DatasourceStatus } from "./queries"
import { PhoneSourceTitle } from "./source-menu"
import { SourceTitle } from "./source-title"
import type { ExploreKind } from "./sources"

function source(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    source: "file",
    healthy: true,
    health_state: "healthy",
    checked_at: "2026-11-19T00:00:00Z",
    dashboard_count: 0,
    ...(["tempo", "jaeger", "clickhouse"].includes(type) && {
      trace_search: true,
    }),
  }
}

const cases: [ExploreKind, string, string][] = [
  ["logs", "loki", "Log sources"],
  ["traces", "tempo", "Trace sources"],
  ["metrics", "prometheus", "Metric sources"],
]

describe("PhoneSourceTitle", () => {
  it.each(cases)(
    "titles the %s switcher sheet %s",
    async (kind, type, title) => {
      const queryClient = testQueryClient()
      queryClient.setQueryData(datasourceKeys.list(), {
        datasources: [source("one", type), source("two", type)],
      })
      const { user } = renderWithProviders(
        <PhoneSourceTitle
          kind={kind}
          label="Page"
          source={source("one", type)}
        />,
        { queryClient, route: { path: "/", url: "/" } }
      )
      await user.click(await screen.findByRole("button", { name: /Switch/ }))
      expect(
        await screen.findByRole("dialog", { name: title })
      ).toBeInTheDocument()
    }
  )

  it("names the SQL mode the page shows, not the datasource's default language", async () => {
    const queryClient = testQueryClient()
    queryClient.setQueryData(datasourceKeys.list(), {
      datasources: [source("ch", "clickhouse")],
    })
    const clickhouse = source("ch", "clickhouse")
    renderWithProviders(
      <>
        <SourceTitle kind="traces" source={clickhouse} language="SQL" />
        <PhoneSourceTitle kind="traces" label="Traces" source={clickhouse} />
      </>,
      { queryClient, route: { path: "/", url: "/" } }
    )
    expect(await screen.findByText("ch · SQL")).toBeInTheDocument()
    expect(screen.queryByText("ch · Search")).toBeNull()
  })

  it("falls back to the datasource's language when no page names one", async () => {
    const clickhouse = source("ch", "clickhouse")
    renderWithProviders(
      <PhoneSourceTitle kind="traces" label="Traces" source={clickhouse} />,
      { route: { path: "/", url: "/" } }
    )
    expect(await screen.findByText("ch · Search")).toBeInTheDocument()
  })
})
