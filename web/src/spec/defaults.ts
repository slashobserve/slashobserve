import { specDefaults } from "./defaults.gen"
import type { DataPanelType } from "./panels"

export type DisplayPanelType = DataPanelType | "dashlist" | "news"

export type DisplayOf<T extends DisplayPanelType> = {
  [K in T]: Record<string, unknown>
}[T]

export const fieldPropDefaults = specDefaults.fieldProps
export const queryDefaults = specDefaults.query
export const variableDefaults = specDefaults.variable
export const rowDefaults = specDefaults.row
export const linkDefaults = specDefaults.link
export const canvasElementDefaults = specDefaults.canvasElement
export const canvasConnectionDefaults = specDefaults.canvasConnection
export const instantPanelTypes: readonly string[] = specDefaults.instantPanels

const displayTable: Record<
  string,
  Record<string, unknown>
> = specDefaults.display

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function copyValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(copyValue)
  if (isPlainObject(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, copyValue(item)])
    )
  }
  return value
}

function displayDefaultsFor(type: string): Record<string, unknown> {
  return Object.hasOwn(displayTable, type) ? displayTable[type] : {}
}

export function displayWithDefaults(
  type: DisplayPanelType,
  display: unknown
): Record<string, unknown> {
  const given = isPlainObject(display)
    ? Object.entries(display).filter(([, value]) => value !== undefined)
    : []
  return Object.fromEntries([
    ...Object.entries(copyValue(displayDefaultsFor(type)) as object),
    ...given,
  ])
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
  if (isPlainObject(a) || isPlainObject(b)) {
    if (!isPlainObject(a) || !isPlainObject(b)) return false
    const keys = Object.keys(a)
    return (
      keys.length === Object.keys(b).length &&
      keys.every((key) => Object.hasOwn(b, key) && sameValue(a[key], b[key]))
    )
  }
  return a === b
}

export function isDefaultDisplayValue(
  type: DisplayPanelType,
  key: string,
  value: unknown
): boolean {
  const defaults = displayDefaultsFor(type)
  return Object.hasOwn(defaults, key) && sameValue(defaults[key], value)
}
