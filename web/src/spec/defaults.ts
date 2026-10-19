import { specDefaults } from "./defaults.gen"
import type { DataPanel } from "./observespec.gen"

type DataPanelType = DataPanel["type"]

export type DisplayOf<T extends DataPanelType> = {
  [K in T]: Record<string, unknown>
}[T]

export const fieldPropDefaults = specDefaults.fieldProps
export const queryDefaults = specDefaults.query
export const variableDefaults = specDefaults.variable
export const rowDefaults = specDefaults.row
export const linkDefaults = specDefaults.link

function copyValue(value: unknown): unknown {
  return Array.isArray(value) ? value.map(copyValue) : value
}

function defaultDisplay(type: DataPanelType): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(specDefaults.display[type]).map(([key, value]) => [
      key,
      copyValue(value),
    ])
  )
}

export function displayWithDefaults(
  type: DataPanelType,
  display: unknown
): Record<string, unknown> {
  const given =
    typeof display === "object" && display !== null && !Array.isArray(display)
      ? display
      : {}
  return { ...defaultDisplay(type), ...given }
}

function sameValue(a: unknown, b: unknown): boolean {
  if (Array.isArray(a) || Array.isArray(b)) {
    return (
      Array.isArray(a) &&
      Array.isArray(b) &&
      a.length === b.length &&
      a.every((item, i) => sameValue(item, b[i]))
    )
  }
  return a === b
}

export function isDefaultDisplayValue(
  type: DataPanelType,
  key: string,
  value: unknown
): boolean {
  const defaults: Record<string, unknown> = specDefaults.display[type]
  return Object.hasOwn(defaults, key) && sameValue(defaults[key], value)
}
