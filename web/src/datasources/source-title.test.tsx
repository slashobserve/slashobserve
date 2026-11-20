import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { TitleHostContext } from "@/shell/top-row-title"

import type { DatasourceStatus } from "./queries"
import { SourceTitle } from "./source-title"

const clickhouse = {
  uid: "ch",
  name: "ClickHouse",
  type: "clickhouse",
} as DatasourceStatus

function renderTitle(language?: string) {
  const host = document.createElement("span")
  document.body.append(host)
  render(
    <TitleHostContext value={host}>
      <SourceTitle kind="traces" source={clickhouse} language={language} />
    </TitleHostContext>
  )
  return host
}

describe("SourceTitle", () => {
  it("names the datasource's language by default", () => {
    expect(renderTitle().textContent).toBe("ClickHouseSearch")
  })

  it("names the language the page says is shown", () => {
    expect(renderTitle("SQL").textContent).toBe("ClickHouseSQL")
    expect(screen.queryByText("Search")).toBeNull()
  })
})
