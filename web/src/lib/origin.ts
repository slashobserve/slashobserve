import type { components } from "@/api/schema.gen"

type Creator = components["schemas"]["Creator"]
type VersionMeta = components["schemas"]["VersionMeta"]

const clientNames = new Map<string, string>(
  Object.entries({
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
  })
)

const unknownClient = "MCP client"

function namedClient(raw: string | null | undefined): string | null {
  const trimmed = raw?.trim() ?? ""
  if (trimmed === "") return null
  return clientNames.get(trimmed.toLowerCase()) ?? trimmed
}

export function clientDisplayName(raw: string | null | undefined): string {
  return namedClient(raw) ?? unknownClient
}

const originNames = new Map<string, string>([
  ["web", "Web"],
  ["cli", "CLI"],
  ["api", "API"],
])

const detailSeparator = " · "

export function madeByChannel(creator: Creator): string | null {
  const isMcp = creator.origin === "mcp"
  const origin = isMcp
    ? clientDisplayName(creator.origin_client)
    : (originNames.get(creator.origin ?? "") ?? null)
  if (creator.via !== "agent") return origin
  return isMcp ? `Agent${detailSeparator}${origin}` : "Agent"
}

export function madeByDetail(creator: Creator): string {
  const email = creator.author.email?.trim() ?? ""
  const identity =
    creator.author.kind === "service_account"
      ? "service account"
      : email === ""
        ? null
        : email
  return [identity, madeByChannel(creator)]
    .filter((part) => part !== null)
    .join(detailSeparator)
}

export function madeByBreadcrumb(creator: Creator): string | null {
  if (creator.origin !== "mcp") return null
  const client = namedClient(creator.origin_client)
  return client === null ? "made by an MCP client" : `made by ${client}`
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
