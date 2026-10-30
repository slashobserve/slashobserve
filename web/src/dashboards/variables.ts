import type { Selections } from "@/api/keys"
import type {
  AdhocVariable,
  CustomVariable,
  DatasourceVariable,
  GroupByVariable,
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

export type ValuesData = {
  values: string[]
  texts?: string[]
  truncated: boolean
}

export function optionTexts(
  data: Pick<ValuesData, "values" | "texts"> | undefined
): (value: string) => string {
  const { values, texts } = data ?? { values: [] }
  if (!texts) return (value) => value
  const byValue = new Map<string, string>()
  values.forEach((value, index) => {
    if (!byValue.has(value)) byValue.set(value, texts[index] ?? value)
  })
  return (value) => byValue.get(value) ?? value
}

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
      return readyDisplay(customOptions(variable))
    case "interval":
      return readyDisplay({
        values: intervalOptions(variable),
        truncated: false,
      })
    case "constant":
    case "textbox":
    case "adhoc":
    case "groupby":
      return { status: "ready", shown: undefined, selectable: undefined }
    default:
      return undefined
  }
}

function customOptions(variable: CustomVariable): ValuesData {
  const values: string[] = []
  const texts: string[] = []
  for (const option of variable.options) {
    values.push(typeof option === "string" ? option : option.value)
    texts.push(typeof option === "string" ? option : option.text)
  }
  const differs = texts.some((text, index) => text !== values[index])
  return { values, ...(differs ? { texts } : {}), truncated: false }
}

function readyDisplay(data: ValuesData): VariableDisplay {
  return { status: "ready", shown: data, selectable: data.values }
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

function declaredSelection(
  variable: QueryVariable | CustomVariable
): string[] | undefined {
  const values = [variable.default ?? []].flat()
  if (values.length === 0) return undefined
  return values.includes(ALL) ? [ALL] : values
}

function savedFilters(variable: AdhocVariable): string[] {
  return (variable.default ?? []).map((filter) =>
    formatFilter({
      key: filter.key,
      op: filter.operator,
      value: filter.value,
    })
  )
}

function adhocSelection(
  variable: AdhocVariable,
  urlValues: string[]
): string[] | undefined {
  const saved = savedFilters(variable)
  if (urlValues.length === 0) return saved.length > 0 ? saved : undefined
  const filters = parseFilters(urlValues)
  if (filters.length > 0) return filters.map(formatFilter)
  return saved.length > 0 ? [""] : undefined
}

export const maxGroupByKeys = 20

const groupByKeyPattern = /^[A-Za-z_][A-Za-z0-9_]{0,199}$/

export function isGroupByKey(text: string): boolean {
  return groupByKeyPattern.test(text)
}

function groupByKeys(urlValues: string[]): string[] {
  return [...new Set(urlValues.filter(isGroupByKey))].slice(0, maxGroupByKeys)
}

function groupBySelection(
  variable: GroupByVariable,
  urlValues: string[]
): string[] | undefined {
  const saved = variable.default ?? []
  if (urlValues.length === 0) return saved.length > 0 ? saved : undefined
  const keys = groupByKeys(urlValues)
  if (keys.length > 0) return keys
  return saved.length > 0 ? [""] : undefined
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
        const selection = adhocSelection(variable, urlValues)
        if (selection) selections[variable.name] = selection
        break
      }
      case "groupby": {
        const selection = groupBySelection(variable, urlValues)
        if (selection) selections[variable.name] = selection
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
        const declared = declaredSelection(variable)
        if (explicit.length > 0) {
          selections[variable.name] = explicit
        } else if (declared) {
          selections[variable.name] = declared
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

export type ValuesRequest = { parents: Selections; parentsReady: boolean }

export function valuesRequests(
  variables: Variable[],
  urlVars: Record<string, string[]>,
  options: VariableOptions
): ValuesRequest[] {
  const resolved = resolveSelections(variables, urlVars, options)
  return variables.map((_, index) => {
    const parents: Selections = {}
    let parentsReady = true
    for (const parent of variables.slice(0, index)) {
      const values = resolved.selections[parent.name]
      if (values) parents[parent.name] = values
      if (resolved.waiting.includes(parent.name)) parentsReady = false
    }
    return { parents: sortSelections(parents), parentsReady }
  })
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
  const variable = variables[index]
  if (values.length > 0) next[name] = values
  else if (variable?.type === "adhoc" && savedFilters(variable).length > 0) {
    next[name] = [""]
  } else if (
    variable?.type === "groupby" &&
    (variable.default ?? []).length > 0
  ) {
    next[name] = [""]
  }
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
