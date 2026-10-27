import type { Selections } from "@/api/keys"
import type {
  CustomVariable,
  DatasourceVariable,
  IntervalVariable,
  QueryVariable,
  Variable,
} from "@/spec"

import { ALL } from "./search"
import { formatFilter, parseFilters } from "./variable-controls/adhoc"

export const AUTO = "$__auto"

export function intervalOptions(variable: IntervalVariable): string[] {
  return variable.auto ? [AUTO, ...variable.options] : variable.options
}

export class VariableLoadError extends Error {
  constructor(label: string) {
    super(`Could not load the values of ${label}`)
    this.name = "VariableLoadError"
  }
}

export type VariableOptions = Record<string, string[] | undefined>

export function sortSelections(selections: Selections): Selections {
  const out: Selections = {}
  for (const name of Object.keys(selections).sort()) {
    out[name] = selections[name]
  }
  return out
}

export function earlierSelections(
  variables: Variable[],
  index: number,
  urlVars: Record<string, string[]>
): Selections {
  const out: Selections = {}
  for (const variable of variables.slice(0, index)) {
    const values = urlVars[variable.name]
    if (values) out[variable.name] = values
  }
  return sortSelections(out)
}

export type ValuesData = { values: string[]; truncated: boolean }

export type LoadedValues = { data: ValuesData; parentKey: string }

export type VariableDisplay = {
  status: "loading" | "refreshing" | "ready" | "error"
  shown: ValuesData | undefined
  selectable: string[] | undefined
}

export function parentKeyOf(selections: Selections): string {
  return JSON.stringify(selections)
}

function localDisplay(variable: Variable): VariableDisplay | undefined {
  switch (variable.type) {
    case "custom":
      return readyDisplay(variable.options)
    case "interval":
      return readyDisplay(intervalOptions(variable))
    case "constant":
    case "textbox":
    case "adhoc":
      return { status: "ready", shown: undefined, selectable: undefined }
    default:
      return undefined
  }
}

function readyDisplay(values: string[]): VariableDisplay {
  return {
    status: "ready",
    shown: { values, truncated: false },
    selectable: values,
  }
}

export function variableDisplay(
  variable: Variable,
  request: {
    data: ValuesData | undefined
    last: LoadedValues | undefined
    parentKey: string
    isError: boolean
    isFetching: boolean
  }
): VariableDisplay {
  const local = localDisplay(variable)
  if (local) return local
  const { last } = request
  const lastValues =
    last?.parentKey === request.parentKey ? last.data.values : undefined
  if (request.isError && !request.isFetching) {
    return {
      status: "error",
      shown: request.data,
      selectable: request.data?.values ?? lastValues,
    }
  }
  if (request.data) {
    return {
      status: "ready",
      shown: request.data,
      selectable: request.data.values,
    }
  }
  if (last && request.isFetching) {
    return { status: "refreshing", shown: last.data, selectable: lastValues }
  }
  return { status: "loading", shown: undefined, selectable: undefined }
}

function fromUrl(
  variable: QueryVariable | CustomVariable,
  values: string[]
): string[] {
  if (variable.includeAll && values.includes(ALL)) return [ALL]
  const concrete = values.filter((value) => value !== ALL && value !== "")
  if (concrete.length === 0) return []
  return variable.multi ? concrete : [concrete[0]]
}

function intervalSelection(
  variable: IntervalVariable,
  urlValue: string | undefined
): string {
  const options = intervalOptions(variable)
  if (urlValue !== undefined && options.includes(urlValue)) return urlValue
  if (variable.default !== undefined && options.includes(variable.default)) {
    return variable.default
  }
  return options[0]
}

function datasourceSelection(
  variable: DatasourceVariable,
  urlValue: string | undefined,
  available: string[]
): string {
  if (urlValue !== undefined && available.includes(urlValue)) return urlValue
  if (variable.default !== undefined && available.includes(variable.default)) {
    return variable.default
  }
  return available[0]
}

export function resolveSelections(
  variables: Variable[],
  urlVars: Record<string, string[]>,
  options: VariableOptions
): { selections: Selections; pending: boolean; waiting: string[] } {
  const selections: Selections = {}
  const waiting: string[] = []
  for (const variable of variables) {
    const urlValues = urlVars[variable.name] ?? []
    switch (variable.type) {
      case "constant":
        selections[variable.name] = [variable.value]
        break
      case "textbox":
        selections[variable.name] = [urlValues[0] ?? variable.default ?? ""]
        break
      case "interval":
        selections[variable.name] = [intervalSelection(variable, urlValues[0])]
        break
      case "adhoc": {
        const filters = parseFilters(urlValues)
        if (filters.length > 0) {
          selections[variable.name] = filters.map(formatFilter)
        }
        break
      }
      case "datasource": {
        const available = options[variable.name]
        if (available === undefined) waiting.push(variable.name)
        else if (available.length > 0) {
          selections[variable.name] = [
            datasourceSelection(variable, urlValues[0], available),
          ]
        }
        break
      }
      default: {
        const explicit = fromUrl(variable, urlValues)
        if (explicit.length > 0) {
          selections[variable.name] = explicit
        } else if (variable.includeAll) {
          selections[variable.name] = [ALL]
        } else {
          const available = options[variable.name]
          if (available === undefined) waiting.push(variable.name)
          else if (available.length > 0) {
            selections[variable.name] = [available[0]]
          }
        }
      }
    }
  }
  return { selections, pending: waiting.length > 0, waiting }
}

function dependsOnEarlier(variable: Variable): boolean {
  return (
    variable.type === "query" ||
    variable.type === "custom" ||
    variable.type === "datasource"
  )
}

export function applyVariableChange(
  variables: Variable[],
  urlVars: Record<string, string[]>,
  name: string,
  values: string[]
): Record<string, string[]> {
  const index = variables.findIndex((variable) => variable.name === name)
  const later = new Set(
    variables
      .slice(index + 1)
      .filter(dependsOnEarlier)
      .map((v) => v.name)
  )
  const next: Record<string, string[]> = {}
  for (const [key, value] of Object.entries(urlVars)) {
    if (key !== name && !later.has(key)) next[key] = value
  }
  if (values.length > 0) next[name] = values
  return next
}

export function normalizeMultiSelection(
  previous: string[],
  next: string[],
  includeAll: boolean
): string[] {
  if (!includeAll) return next
  if (next.includes(ALL) && !previous.includes(ALL)) return [ALL]
  return next.filter((value) => value !== ALL)
}
