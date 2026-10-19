export function commentsListCommand(dashboardId: string): string {
  return `observe comments list ${dashboardId}`
}

export function claudeCodePrompt(input: {
  dashboardId: string
  title: string
  url: string
}): string {
  return [
    `Work through the open comments on the SlashObserve dashboard "${input.title}" (${input.url}).`,
    `List them with the SlashObserve MCP tool list_comments, or run: ${commentsListCommand(input.dashboardId)}`,
    "For each one, change the dashboard with update_dashboard, reply with reply_to_comment, and resolve it with resolve_comment, passing the version you saved.",
  ].join("\n")
}
