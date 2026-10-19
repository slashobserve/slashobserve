import {
  matchesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type OrganizeOptions = {
  exclude?: string[]
  rename?: Record<string, string>
  order?: string[]
}

type Kept = { original: PreparedField; renamed: PreparedField }

function renameOf(
  frame: PreparedFrame,
  field: PreparedField,
  rename: Record<string, string>
): string | undefined {
  for (const [from, to] of Object.entries(rename)) {
    if (matchesName(frame, field, from)) return to
  }
  return undefined
}

function organizeFrame(
  frame: PreparedFrame,
  { exclude = [], rename = {}, order = [] }: OrganizeOptions
): PreparedFrame {
  const kept: Kept[] = frame.fields
    .filter((field) => !exclude.some((name) => matchesName(frame, field, name)))
    .map((field) => {
      const name = renameOf(frame, field, rename)
      return {
        original: field,
        renamed: name ? { ...field, name, seriesName: name } : field,
      }
    })
  const placed = new Set<Kept>()
  const first: Kept[] = []
  for (const name of order) {
    for (const item of kept) {
      if (placed.has(item)) continue
      if (
        matchesName(frame, item.original, name) ||
        item.renamed.name === name
      ) {
        placed.add(item)
        first.push(item)
      }
    }
  }
  const rest = kept.filter((item) => !placed.has(item))
  return withFields(
    frame,
    [...first, ...rest].map((item) => item.renamed)
  )
}

export function organize(
  frames: PreparedFrame[],
  options: OrganizeOptions
): PreparedFrame[] {
  return frames.map((frame) => organizeFrame(frame, options))
}
