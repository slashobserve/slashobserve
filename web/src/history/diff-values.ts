import { queryRef, type PanelQuery } from "@/spec"

import type { TitleMaps } from "./diff-groups"

export type ValueBlock = {
  label?: string
  text: string
  options: string
}

function isQueryEntry(value: unknown): value is PanelQuery {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false
  }
  const { query, ref } = value as { query?: unknown; ref?: unknown }
  return (
    typeof query === "string" && (ref === undefined || typeof ref === "string")
  )
}

function queryEntries(field: string, value: unknown): PanelQuery[] | null {
  if (
    field === "queries" &&
    Array.isArray(value) &&
    value.length > 0 &&
    value.every(isQueryEntry)
  ) {
    return value
  }
  return null
}

function scalar(value: unknown): string {
  return typeof value === "string" ? value : (JSON.stringify(value) ?? "")
}

function queryOptions(entry: PanelQuery): string {
  return Object.entries(entry)
    .filter(([key]) => key !== "ref" && key !== "query")
    .flatMap(([key, value]) => {
      if (value === false || value === undefined) return []
      if (value === true) return [key === "hide" ? "hidden" : key]
      return [`${key}: ${scalar(value)}`]
    })
    .join(", ")
}

export function shouldLabelQueries(
  field: string,
  reorder: boolean,
  ...values: unknown[]
): boolean {
  if (reorder) return false
  return values.some((value) => {
    const entries = queryEntries(field, value)
    return (
      entries !== null &&
      (entries.length > 1 || entries.some((entry) => hasExplicitRef(entry)))
    )
  })
}

function hasExplicitRef(entry: PanelQuery): boolean {
  return entry.ref !== undefined && entry.ref !== ""
}

function queryBlocks(entries: PanelQuery[], label: boolean): ValueBlock[] {
  return entries.map((entry, index) => ({
    label: label
      ? `Query ${queryRef(hasExplicitRef(entry) ? entry : { ...entry, ref: undefined }, index)}`
      : undefined,
    text: entry.query,
    options: queryOptions(entry),
  }))
}

function reorderText(value: unknown[], titles: TitleMaps): string {
  return value
    .map((item, index) => {
      const id = String(item)
      const title =
        (Object.hasOwn(titles.to, id) ? titles.to[id] : undefined) ??
        (Object.hasOwn(titles.from, id) ? titles.from[id] : undefined) ??
        id
      return `${index + 1}. ${title}`
    })
    .join("\n")
}

function plainText(value: unknown): string {
  if (typeof value === "string") return value
  if (value === undefined) return ""
  return JSON.stringify(value, null, 2) ?? ""
}

export function formatValue(
  field: string,
  value: unknown,
  titles: TitleMaps,
  reorder: boolean,
  labelQueries: boolean
): ValueBlock[] {
  if (reorder && Array.isArray(value)) {
    return [{ text: reorderText(value, titles), options: "" }]
  }
  const entries = queryEntries(field, value)
  if (entries !== null) return queryBlocks(entries, labelQueries)
  return [{ text: plainText(value), options: "" }]
}

export function limitBlocks(
  blocks: ValueBlock[],
  limit: number
): { blocks: ValueBlock[]; cut: boolean } {
  const shown: ValueBlock[] = []
  let remaining = limit
  for (const block of blocks) {
    if (remaining === 0) return { blocks: shown, cut: true }
    const { text, options } = block
    if (text.length + options.length <= remaining) {
      shown.push(block)
      remaining -= text.length + options.length
      continue
    }
    if (text.length <= remaining) {
      shown.push({
        ...block,
        options: `${options.slice(0, remaining - text.length)}...`,
      })
    } else {
      const keptOptions = Math.min(options.length, remaining)
      shown.push({
        ...block,
        text: `${text.slice(0, remaining - keptOptions)}...`,
        options:
          options.length > remaining
            ? `${options.slice(0, remaining)}...`
            : options,
      })
    }
    return { blocks: shown, cut: true }
  }
  return { blocks: shown, cut: false }
}
