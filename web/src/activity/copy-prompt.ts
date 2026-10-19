export function commentsListCommand(dashboardId: string): string {
  return `observe comments list ${dashboardId}`
}

const hiddenCharacters = /[\p{Cc}\u200E\u200F\u202A-\u202E\u2066-\u2069]/gu

function plainTitle(title: string): string {
  return title
    .replace(/\s+/g, " ")
    .replace(hiddenCharacters, "")
    .replace(/ {2,}/g, " ")
    .trim()
}

export function claudeCodePrompt(input: {
  dashboardId: string
  title: string
  url: string
}): string {
  return [
    `Work through the open comments on the SlashObserve dashboard "${plainTitle(input.title)}" (${input.url}).`,
    `List them with the SlashObserve MCP tool list_comments, or run: ${commentsListCommand(input.dashboardId)}`,
    "For each one, change the dashboard with update_dashboard, reply with reply_to_comment, and resolve it with resolve_comment, passing the version you saved.",
  ].join("\n")
}
