export type SlashCommand = "investigate" | "import" | "revise"

export const commandLabels: Record<SlashCommand, string> = {
  investigate: "Investigate",
  import: "Import",
  revise: "Revise",
}

export const commandHints: Record<SlashCommand, string> = {
  investigate: "asks a question and answers with a finding",
  import: "imports a Grafana dashboard from a link or its JSON",
  revise: "changes this dashboard",
}

const allCommands: readonly SlashCommand[] = ["investigate", "import", "revise"]

function isAvailable(
  name: string,
  available: readonly SlashCommand[]
): name is SlashCommand {
  return (available as readonly string[]).includes(name)
}

export function takeCommand(
  text: string,
  available: readonly SlashCommand[]
): { command: SlashCommand; rest: string } | null {
  const match = /^\/([a-z]+)\s([\s\S]*)$/.exec(text)
  if (match === null || !isAvailable(match[1], available)) return null
  return { command: match[1], rest: match[2].trimStart() }
}

export function matchingCommands(
  text: string,
  available: readonly SlashCommand[]
): SlashCommand[] {
  const match = /^\/([a-z]*)$/.exec(text)
  if (match === null) return []
  return available.filter((name) => name.startsWith(match[1]))
}

export function withCommand(
  command: SlashCommand | null,
  text: string
): string {
  return command === null ? text : `/${command} ${text}`
}

export function availableCommands(input: {
  hasDashboard: boolean
  surface: "chat" | "activity"
  canInvestigate: boolean
}): SlashCommand[] {
  return allCommands.filter((name) => {
    if (name === "investigate") return input.canInvestigate
    if (name === "import") return input.surface === "chat"
    return input.hasDashboard
  })
}
