import type { components } from "@/api/schema.gen"

type Creator = components["schemas"]["Creator"]
type VersionMeta = components["schemas"]["VersionMeta"]

const channelNames = new Map<string, string>([
  ["web", "Web"],
  ["cli", "CLI"],
  ["api", "API"],
])

const detailSeparator = " · "

export function madeByChannel(creator: Pick<Creator, "actor">): string | null {
  const { run_id, origin } = creator.actor
  if (run_id !== null || origin === "mcp") return null
  return channelNames.get(origin ?? "") ?? null
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

export function madeByBreadcrumb(
  creator: Pick<Creator, "actor">
): string | null {
  if (creator.actor.origin !== "mcp") return null
  const { client } = creator.actor
  return client === null ? "made by an MCP client" : `made by ${client}`
}

type Actor = components["schemas"]["Actor"]

export function isAgentActor(actor: Pick<Actor, "run_id" | "origin">): boolean {
  return actor.run_id !== null || actor.origin === "mcp"
}

export type VersionActor = { who: string; verb: string; agent: boolean }

export function versionActor(meta: Pick<VersionMeta, "actor">): VersionActor {
  return {
    who: meta.actor.label,
    verb: "saved",
    agent: isAgentActor(meta.actor),
  }
}
