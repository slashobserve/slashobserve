export function examplePrompts(types: readonly string[]): string[] {
  const prom = types.includes("prometheus")
  const ch = types.includes("clickhouse")
  const prompts: string[] = []
  if (prom) prompts.push("Service health by pod, last 6h")
  if (ch) prompts.push("Error logs by service from ClickHouse")
  if (prom) prompts.push("Node CPU and memory", "p95 latency by route")
  if (ch && !prom) {
    prompts.push(
      "Slowest traces by service from ClickHouse",
      "Log volume by severity from ClickHouse"
    )
  }
  return prompts.slice(0, 4)
}
