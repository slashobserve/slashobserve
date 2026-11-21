import type { Selections } from "@/api/keys"
import { ALL } from "@/dashboards/search"
import type { VariableState } from "@/dashboards/use-variables"
import {
  resolveSelections,
  variableDisplay,
  type VariableOptions,
} from "@/dashboards/variables"
import { resolveTime } from "@/lib/time"
import type { DataPanel, Variable } from "@/spec"

const variableName = /\$\{?(?!__)([A-Za-z_]\w*)/g

function collectNames(value: unknown, into: Set<string>) {
  if (typeof value === "string") {
    for (const match of value.matchAll(variableName)) into.add(match[1])
  } else if (Array.isArray(value)) {
    for (const item of value) collectNames(item, into)
  } else if (typeof value === "object" && value !== null) {
    for (const item of Object.values(value)) collectNames(item, into)
  }
}

function withSelection(variable: Variable, values: string[] | undefined) {
  if (values === undefined || values.length === 0) return variable
  switch (variable.type) {
    case "query":
    case "custom": {
      const all = values.includes(ALL)
      const chosen = all ? [ALL] : values
      return {
        ...variable,
        default: variable.multi || all ? chosen : chosen[0],
      } as Variable
    }
    case "textbox":
    case "interval":
    case "datasource":
      return { ...variable, default: values[0] } as Variable
    default:
      return variable
  }
}

export function blockVariables(
  panel: DataPanel,
  variables: readonly Variable[],
  selections: Selections
): Variable[] {
  const wanted = new Set<string>()
  collectNames(panel.queries, wanted)
  collectNames(panel.datasource, wanted)
  const included = new Set<string>()
  let grew = true
  while (grew) {
    grew = false
    for (const variable of variables) {
      if (included.has(variable.name) || !wanted.has(variable.name)) continue
      included.add(variable.name)
      collectNames(variable, wanted)
      grew = true
    }
  }
  return variables
    .filter((variable) => included.has(variable.name))
    .map((variable) => withSelection(variable, selections[variable.name]))
}

export function bakedSelections(
  variables: readonly Variable[] | undefined
): Selections {
  const out: Selections = {}
  for (const variable of variables ?? []) {
    const chosen = "default" in variable ? variable.default : undefined
    if (typeof chosen === "string") out[variable.name] = [chosen]
    else if (
      Array.isArray(chosen) &&
      chosen.every((v) => typeof v === "string")
    )
      out[variable.name] = chosen
    else if (variable.type === "constant") out[variable.name] = [variable.value]
  }
  return out
}

function bakedValues(variable: Variable): string[] {
  return bakedSelections([variable])[variable.name] ?? []
}

function ignoreRetry() {}

export function blockVariableStates(
  variables: readonly Variable[],
  chosen: Selections
): { states: VariableState[]; selections: Selections } {
  const displays = variables.map((variable) =>
    variableDisplay(variable, {
      data: { values: bakedValues(variable), truncated: false },
      last: undefined,
      parentKey: "",
      isError: false,
      isFetching: false,
    })
  )
  const options: VariableOptions = {}
  variables.forEach((variable, index) => {
    options[variable.name] = displays[index].selectable
  })
  const { selections } = resolveSelections([...variables], chosen, options)
  const states = variables.map((variable, index) => ({
    variable,
    options: displays[index].shown?.values,
    texts: displays[index].shown?.texts,
    truncated: false,
    status: displays[index].status,
    selected: selections[variable.name] ?? [],
    retry: ignoreRetry,
  }))
  return { states, selections }
}

export function variableCaption(selections: Selections): string {
  const parts = Object.entries(selections).map(
    ([name, values]) =>
      `${name}=${values.map((v) => (v === ALL ? "All" : v)).join(",")}`
  )
  return parts.length === 0 ? "" : `with ${parts.join(", ")}`
}

export function blockRange(
  from: string | undefined,
  to: string | undefined,
  now: number
): { from: string | null; to: string | null } {
  const none = { from: null, to: null }
  if (from === undefined || to === undefined) return none
  const start = resolveTime(from, now, false)
  const end = resolveTime(to, now, true)
  if (Number.isNaN(start) || Number.isNaN(end) || start >= end) return none
  return {
    from: new Date(start).toISOString(),
    to: new Date(end).toISOString(),
  }
}

export function blockPanelFrom(panel: DataPanel): DataPanel {
  const copy = { ...panel }
  delete copy.repeat
  return copy
}
