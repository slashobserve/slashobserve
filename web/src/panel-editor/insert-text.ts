function escapeWith(value: string, quote: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll(quote, `\\${quote}`)
}

export function metricInsert(type: string | undefined, name: string): string {
  if (type === "clickhouse") return `MetricName = '${escapeWith(name, "'")}'`
  return name
}

export function matcherInsert(
  type: string | undefined,
  label: string,
  value: string
): string {
  if (type === "clickhouse") return `${label} = '${escapeWith(value, "'")}'`
  return `${label}="${escapeWith(value, '"')}"`
}
