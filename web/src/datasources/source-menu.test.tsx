import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { DatasourceStatus } from "./queries"
import { PhoneSourceTitle } from "./source-menu"
import type { ExploreKind } from "./sources"

function source(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "2026-11-19T00:00:00Z",
    dashboard_count: 0,
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
})
