import type { components } from "@/api/schema.gen"

export type ConnectedAgent = components["schemas"]["ConnectedAgent"]

export type GrantScope = "mine" | "all"

export type GrantMode = ConnectedAgent["mode"]

export const modeChoices: {
  value: GrantMode
  label: string
  description: string
}[] = [
  {
    value: "direct",
    label: "Direct",
    description: "Saves change dashboards",
  },
  {
    value: "propose",
    label: "Propose",
    description: "Saves become proposals an editor merges",
  },
]

export function modeLabel(mode: GrantMode): string {
  return modeChoices.find((choice) => choice.value === mode)?.label ?? mode
}

export function agentName(name: string | null): string {
  return name ?? "Unnamed MCP client"
}

export function disconnectTitle(name: string | null): string {
  return `Disconnect ${agentName(name)}?`
}

export function disconnectLabel(
  grant: ConnectedAgent,
  scope: GrantScope
): string {
  const name = agentName(grant.client.name)
  return scope === "all"
    ? `Disconnect ${name} of ${grant.person.name}`
    : `Disconnect ${name}`
}

export function connectPrompt(count: number): "none" | "another" {
  return count === 0 ? "none" : "another"
}
