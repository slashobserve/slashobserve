import { claudeOAuthSnippet } from "@/connected-agents/snippets"

export type ConnectClient = {
  value: string
  label: string
  snippet: (mcpUrl: string) => string
}

export const connectClients: readonly ConnectClient[] = [
  { value: "claude-code", label: "Claude Code", snippet: claudeOAuthSnippet },
]

export function isNewConnection(
  before: readonly string[],
  after: readonly string[]
): string | null {
  const known = new Set(before)
  return after.find((id) => !known.has(id)) ?? null
}
