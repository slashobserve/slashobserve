const plainPromName = /^[a-zA-Z_:][a-zA-Z0-9_:]*$/

function escapeWith(value: string, quote: string): string {
  return value
    .replaceAll("\\", "\\\\")
    .replaceAll(quote, `\\${quote}`)
    .replaceAll("\n", "\\n")
}

function promString(value: string): string {
  return `"${escapeWith(value, '"')}"`
}

export function metricInsert(type: string | undefined, name: string): string {
  if (type === "clickhouse") return `MetricName = '${escapeWith(name, "'")}'`
  return plainPromName.test(name) ? name : `{${promString(name)}}`
}

export function matcherInsert(
  type: string | undefined,
  label: string,
  value: string
): string {
  if (type === "clickhouse") return `${label} = '${escapeWith(value, "'")}'`
  const name = plainPromName.test(label) ? label : promString(label)
  return `${name}=${promString(value)}`
}
