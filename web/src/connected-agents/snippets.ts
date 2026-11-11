export function claudeOAuthSnippet(mcpUrl: string): string {
  return `claude mcp add --transport http slashobserve ${mcpUrl}`
}
