export type Example = { label: string; prompt: string; dashboardId?: string }

const maxExamples = 4
const maxTitleLength = 40

function clipTitle(title: string): string {
  const characters = [...title]
  return characters.length > maxTitleLength
    ? `${characters.slice(0, maxTitleLength).join("")}...`
    : title
}

function example(text: string, dashboardId?: string): Example {
  return dashboardId === undefined
    ? { label: text, prompt: text }
    : { label: text, prompt: text, dashboardId }
}

export function examplePrompts(input: {
  types: readonly string[]
  dashboards: readonly { id: string; title: string }[]
}): Example[] {
  const prom = input.types.includes("prometheus")
  const logs = input.types.includes("loki")
  const ch = input.types.includes("clickhouse")
  const traces = input.types.includes("tempo")
  const first = input.dashboards.at(0)
  const examples: Example[] = []
  if (prom && first !== undefined) {
    examples.push(
      example(`Add a latency row to ${clipTitle(first.title)}`, first.id)
    )
  }
  if (prom) examples.push(example("Service health by pod, last 6h"))
  if (logs || ch) examples.push(example("Error logs by service, last hour"))
  if (traces || (ch && !prom)) {
    examples.push(example("Slowest traces by service, last hour"))
  }
  if (prom) examples.push(example("p95 latency by route"))
  return examples.slice(0, maxExamples)
}
