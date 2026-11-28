export type SlashCommand = "investigate" | "import"

export const commandLabels: Record<SlashCommand, string> = {
  investigate: "Investigate",
  import: "Import",
}

export const commandHints: Record<SlashCommand, string> = {
  investigate: "asks a question and answers with a finding",
  import: "imports a Grafana dashboard from a link or its JSON",
}

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
  if (match === null) return null
  const [, name, rest] = match
  if (name === undefined || rest === undefined || !isAvailable(name, available))
    return null
  return { command: name, rest: rest.trimStart() }
}

export function matchingCommands(
  text: string,
  available: readonly SlashCommand[]
): SlashCommand[] {
  const match = /^\/([a-z]*)$/.exec(text)
  const prefix = match?.[1]
  if (prefix === undefined) return []
  return available.filter((name) => name.startsWith(prefix))
}

export function withCommand(
  command: SlashCommand | null,
  text: string
): string {
  return command === null ? text : `/${command} ${text}`
}

export function chatCommands(canInvestigate: boolean): SlashCommand[] {
  return canInvestigate ? ["investigate", "import"] : ["import"]
}
