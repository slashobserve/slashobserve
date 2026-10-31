import type { AdhocFilter as SpecAdhocFilter } from "@/spec"

const adhocOps = ["=", "!=", "=~", "!~"] as const

const orderingOps = ["<", ">"] as const

const allOps = [...adhocOps, ...orderingOps] as const

export type AdhocOp = (typeof allOps)[number]

export type AdhocFilter = {
  key: string
  op: AdhocOp
  value: string
  condition?: "or"
}

export type FilterLabels = { keyLabel?: string; valueLabel?: string }

export const maxAdhocFilters = 20

const orPrefix = "|"

const labelNamePattern = /^[A-Za-z_][A-Za-z0-9_]*$/
const columnPattern = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/
const numberPattern = /^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$/

function isOp(text: string): text is AdhocOp {
  return (allOps as readonly string[]).includes(text)
}

export function opsFor(datasourceType: string | undefined): readonly AdhocOp[] {
  return datasourceType === "clickhouse" ? allOps : adhocOps
}

export function parseFilter(text: string): AdhocFilter | null {
  const or = text.startsWith(orPrefix)
  const body = or ? text.slice(orPrefix.length) : text
  const first = body.indexOf("|")
  if (first < 0) return null
  const second = body.indexOf("|", first + 1)
  if (second < 0) return null
  const key = body.slice(0, first)
  const op = body.slice(first + 1, second)
  if (key === "" || !isOp(op)) return null
  const filter: AdhocFilter = { key, op, value: body.slice(second + 1) }
  return or ? { ...filter, condition: "or" } : filter
}

export function formatFilter(filter: AdhocFilter): string {
  const text = `${filter.key}|${filter.op}|${filter.value}`
  return filter.condition === "or" ? orPrefix + text : text
}

export function describeFilter(
  filter: AdhocFilter,
  labels: FilterLabels = {}
): string {
  return `${labels.keyLabel ?? filter.key} ${filter.op} ${labels.valueLabel ?? filter.value}`
}

export function fromSpecFilter(filter: SpecAdhocFilter): AdhocFilter {
  const out: AdhocFilter = {
    key: filter.key,
    op: filter.operator,
    value: filter.value,
  }
  return filter.condition === "or" ? { ...out, condition: "or" } : out
}

export function labelsBySavedFilter(
  saved: readonly SpecAdhocFilter[] | undefined
): Map<string, FilterLabels> {
  const out = new Map<string, FilterLabels>()
  for (const filter of saved ?? []) {
    if (filter.keyLabel === undefined && filter.valueLabel === undefined) {
      continue
    }
    out.set(formatFilter(fromSpecFilter(filter)), {
      keyLabel: filter.keyLabel,
      valueLabel: filter.valueLabel,
    })
  }
  return out
}

export function parseFilters(texts: string[]): AdhocFilter[] {
  const out: AdhocFilter[] = []
  for (const text of texts) {
    const filter = parseFilter(text)
    if (filter) out.push(filter)
    if (out.length === maxAdhocFilters) break
  }
  return out
}

export function validKey(key: string, datasourceType: string | undefined) {
  const pattern =
    datasourceType === "clickhouse" ? columnPattern : labelNamePattern
  return pattern.test(key)
}

export function keyHint(datasourceType: string | undefined): string {
  return datasourceType === "clickhouse"
    ? "Use a column name."
    : "Use a label name."
}

export function validFilter(
  filter: AdhocFilter,
  datasourceType: string | undefined
): boolean {
  return (
    opsFor(datasourceType).includes(filter.op) &&
    validKey(filter.key, datasourceType)
  )
}

export function compareValues(cell: string, value: string): number {
  if (numberPattern.test(cell) && numberPattern.test(value)) {
    return Math.sign(Number(cell) - Number(value))
  }
  if (cell < value) return -1
  return cell > value ? 1 : 0
}
