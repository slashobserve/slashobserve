export type Example = { label: string; prompt: string; dashboardId?: string }

export type ExampleDashboard = {
  id: string
  title: string
  canEdit: boolean
}

export const noDashboards: readonly ExampleDashboard[] = []

const maxBuildExamples = 3
const maxQuestionExamples = 2
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
  dashboards: readonly ExampleDashboard[]
}): Example[] {
  const prom = input.types.includes("prometheus")
  const logs = input.types.includes("loki")
  const ch = input.types.includes("clickhouse")
  const traces = input.types.includes("tempo")
  const first = input.dashboards.find((dashboard) => dashboard.canEdit)
  const builds: Example[] = []
  if (prom && first !== undefined) {
    builds.push(
      example(`Add a latency row to ${clipTitle(first.title)}`, first.id)
    )
  }
  if (prom) builds.push(example("Service health by pod, last 6h"))
  if (logs || ch) builds.push(example("Error logs by service, last hour"))
  if (traces || (ch && !prom)) {
    builds.push(example("Slowest traces by service, last hour"))
  }
  if (prom) builds.push(example("p95 latency by route"))
  const questions: Example[] = []
  if (prom || logs || ch) {
    questions.push(
      example("Which services had the most errors in the last hour?")
    )
  }
  if (prom || traces) {
    questions.push(example("Which routes are slowest right now?"))
  }
  return [
    ...builds.slice(0, maxBuildExamples),
    ...questions.slice(0, maxQuestionExamples),
  ]
}
