const claudeNoteMarker = "For SlashObserve dashboards"

const claudeNote = `${claudeNoteMarker}: first read open comments with the slashobserve list_comments tool, fix them with update_dashboard, then reply to each comment with reply_to_comment and resolve it with resolve_comment, passing the version that fixed it.`

export function claudeSnippet(mcpUrl: string): string {
  return [
    `claude mcp add --transport http --scope project slashobserve ${mcpUrl} \\`,
    `  --header 'Authorization: Bearer \${OBSERVE_TOKEN}'`,
    `grep -qF '${claudeNoteMarker}' CLAUDE.md 2>/dev/null || echo '${claudeNote}' >> CLAUDE.md`,
  ].join("\n")
}

export function openCodeSnippet(mcpUrl: string): string {
  return [
    `"mcp": {`,
    `  "slashobserve": {`,
    `    "type": "remote",`,
    `    "url": "${mcpUrl}",`,
    `    "oauth": false,`,
    `    "headers": { "Authorization": "Bearer {env:OBSERVE_TOKEN}" }`,
    `  }`,
    `}`,
  ].join("\n")
}

export function cliSnippet(publicUrl: string): string {
  return `export OBSERVE_SERVER=${publicUrl}\nobserve "checkout health by pod, last 6h"`
}
