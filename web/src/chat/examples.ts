export type Example = { label: string; prompt: string; dashboardId?: string }

export type ExampleDashboard = {
  id: string
  title: string
  canEdit: boolean
  updatedBy: string
}

export const noDashboards: readonly ExampleDashboard[] = []

const maxExamples = 4
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

function dashboardToRevise(
  dashboards: readonly ExampleDashboard[],
  personId: string
): ExampleDashboard | undefined {
  const editable = dashboards.filter((dashboard) => dashboard.canEdit)
  return (
    editable.find((dashboard) => dashboard.updatedBy === personId) ??
    editable[0]
  )
}

type SourceTypes = { types: readonly string[] }

export function questionPrompts(input: SourceTypes): Example[] {
  const prom = input.types.includes("prometheus")
  const logs = input.types.includes("loki")
  const ch = input.types.includes("clickhouse")
  const traces = input.types.includes("tempo") || input.types.includes("jaeger")
  const questions: Example[] = []
  if (prom || logs || ch) {
    questions.push(
      example("Which services had the most errors in the last hour?")
    )
  }
  if (prom || traces) {
    questions.push(example("Which routes are slowest right now?"))
  }
  return questions.slice(0, maxQuestionExamples)
}

export function examplePrompts(
  input: SourceTypes & {
    dashboards: readonly ExampleDashboard[]
    canBuild: boolean
    personId: string
  }
): Example[] {
  const prom = input.types.includes("prometheus")
  const logs = input.types.includes("loki")
  const ch = input.types.includes("clickhouse")
  const traces = input.types.includes("tempo") || input.types.includes("jaeger")
  const first = dashboardToRevise(input.dashboards, input.personId)
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
  const asked = questionPrompts(input)
  return [
    ...(input.canBuild ? builds.slice(0, maxExamples - asked.length) : []),
    ...asked,
  ]
}
