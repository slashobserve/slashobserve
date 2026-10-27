export const adhocOps = ["=", "!=", "=~", "!~"] as const

export type AdhocOp = (typeof adhocOps)[number]

export type AdhocFilter = { key: string; op: AdhocOp; value: string }

export const maxAdhocFilters = 20

const labelNamePattern = /^[A-Za-z_][A-Za-z0-9_]*$/
const columnPattern = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/

function isOp(text: string): text is AdhocOp {
  return (adhocOps as readonly string[]).includes(text)
}

export function parseFilter(text: string): AdhocFilter | null {
  const first = text.indexOf("|")
  if (first < 0) return null
  const second = text.indexOf("|", first + 1)
  if (second < 0) return null
  const key = text.slice(0, first)
  const op = text.slice(first + 1, second)
  if (key === "" || !isOp(op)) return null
  return { key, op, value: text.slice(second + 1) }
}

export function formatFilter(filter: AdhocFilter): string {
  return `${filter.key}|${filter.op}|${filter.value}`
}

export function describeFilter(filter: AdhocFilter): string {
  return `${filter.key} ${filter.op} ${filter.value}`
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
  return isOp(filter.op) && validKey(filter.key, datasourceType)
}
