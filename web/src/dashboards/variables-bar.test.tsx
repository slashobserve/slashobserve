import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import type { Variable } from "@/spec"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { VariableState } from "./use-variables"
import { VariablesBar } from "./variables-bar"

function ready(variable: Variable, options: string[]): VariableState {
  return {
    variable,
    options,
    texts: undefined,
    truncated: false,
    status: "ready",
    selected: options.slice(0, 1),
    retry: () => undefined,
  }
}

const states = [
  ready({ name: "env", type: "custom", options: ["prod", "dev"] }, [
    "prod",
    "dev",
  ]),
  ready(
    { name: "pod", label: "Pod", type: "custom", options: ["checkout-0"] },
    ["checkout-0"]
  ),
  ready({ name: "region", type: "constant", value: "eu" }, ["eu"]),
]

function renderBar(onSelect = vi.fn()) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  const result = renderWithProviders(
    <VariablesBar states={states} unverified={new Map()} onSelect={onSelect} />,
    { queryClient }
  )
  return { ...result, onSelect }
}

describe("VariablesBar", () => {
  it("renders a select per shown variable", () => {
    renderBar()
    expect(
      screen
        .getAllByRole("combobox")
        .map((select) => select.getAttribute("aria-label"))
    ).toEqual(["env", "Pod"])
    expect(screen.getByRole("combobox", { name: "env" })).toHaveTextContent(
      "prod"
    )
  })

  it("calls the change handler with the chosen value", async () => {
    const { user, onSelect } = renderBar()
    await user.click(screen.getByRole("combobox", { name: "env" }))
    await user.click(await screen.findByRole("option", { name: "dev" }))
    expect(onSelect).toHaveBeenCalledExactlyOnceWith("env", ["dev"])
  })
})
