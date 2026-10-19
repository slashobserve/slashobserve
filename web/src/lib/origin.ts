import type { components } from "@/api/schema.gen"

type Creator = components["schemas"]["Creator"]
type VersionMeta = components["schemas"]["VersionMeta"]

const clientNames: Record<string, string> = {
  "claude-code": "Claude Code",
  "claude code": "Claude Code",
  "claude-ai": "Claude",
  cursor: "Cursor",
  "cursor-vscode": "Cursor",
  opencode: "OpenCode",
  codex: "Codex",
  "codex-mcp-client": "Codex",
  windsurf: "Windsurf",
  zed: "Zed",
  vscode: "VS Code",
  "visual studio code": "VS Code",
}

const unknownClient = "MCP client"

export function clientDisplayName(raw: string | null | undefined): string {
  const trimmed = raw?.trim() ?? ""
  if (trimmed === "") return unknownClient
  return clientNames[trimmed.toLowerCase()] ?? trimmed
}

export function madeByLabel(creator: Creator): string {
  if (creator.via === "agent") return `Agent, for ${creator.author.name}`
  if (creator.origin === "mcp") {
    return `${clientDisplayName(creator.origin_client)}, ${creator.author.name}`
  }
  return creator.author.name
}

export function madeByBreadcrumb(creator: Creator): string | null {
  if (creator.origin !== "mcp") return null
  const client = creator.origin_client?.trim() ?? ""
  return client === ""
    ? "made by an MCP client"
    : `made by ${clientDisplayName(client)}`
}

export type VersionActor = { who: string; verb: string; agent: boolean }

export function versionActor(
  meta: Pick<
    VersionMeta,
    "author" | "origin" | "origin_client" | "agent_request_id"
  >
): VersionActor {
  if (meta.agent_request_id !== null) {
    return { who: "Agent", verb: "saved", agent: true }
  }
  if (meta.origin === "mcp") {
    return {
      who: clientDisplayName(meta.origin_client),
      verb: "saved over MCP",
      agent: true,
    }
  }
  return { who: meta.author.name, verb: "saved", agent: false }
}
