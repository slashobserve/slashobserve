import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { DatasourceListProvider } from "@/datasources/queries"
import { QueryClientProvider } from "@tanstack/react-query"

import { testQueryClient } from "@/test/render"

import { DatasourceSelect } from "./datasource-select"

const datasources = [
  { uid: "prom", type: "prometheus" },
  { uid: "ch", type: "clickhouse" },
]

function renderSelect(props: {
  value: string
  includePanelDefault?: string
  disabled?: boolean
}) {
  const onChange = vi.fn()
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={testQueryClient()}>
      <DatasourceListProvider value={datasources}>
        <DatasourceSelect
          ariaLabel="Datasource for query A"
          onChange={onChange}
          variables={[
            { name: "ds", type: "datasource", datasourceType: "prometheus" },
          ]}
          {...props}
        />
      </DatasourceListProvider>
    </QueryClientProvider>
  )
  return {
    onChange,
    user,
    trigger: screen.getByRole("combobox", { name: "Datasource for query A" }),
  }
}

describe("DatasourceSelect", () => {
  it("shows the panel datasource for an empty value and reports it as empty", async () => {
    const { trigger, user, onChange } = renderSelect({
      value: "ch",
      includePanelDefault: "prom",
    })
    expect(trigger).toHaveTextContent("ch (ClickHouse)")
    await user.click(trigger)
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "Panel datasource (prom)",
      "$ds (variable, Prometheus)",
      "prom (Prometheus)",
      "ch (ClickHouse)",
    ])
    await user.click(options[0])
    expect(onChange).toHaveBeenCalledExactlyOnceWith("")
  })

  it("asks for a choice when nothing is set and no panel default is offered", () => {
    expect(renderSelect({ value: "" }).trigger).toHaveTextContent(
      "Choose a datasource"
    )
  })

  it("keeps an unknown value selectable and passes a chosen uid through", async () => {
    const { trigger, user, onChange } = renderSelect({ value: "gone" })
    expect(trigger).toHaveTextContent("gone")
    await user.click(trigger)
    await user.click(
      await screen.findByRole("option", { name: "prom (Prometheus)" })
    )
    expect(onChange).toHaveBeenCalledExactlyOnceWith("prom")
  })
})
