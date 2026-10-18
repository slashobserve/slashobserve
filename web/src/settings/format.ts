import type { Instance } from "@/api/instance"
import { domainList } from "@/people/sign-in-policy"

const providerNames: Record<string, string> = {
  anthropic: "Anthropic",
  openai: "OpenAI-compatible",
  replay: "Replay",
}

const effortNames: Record<string, string> = {
  xhigh: "Extra high",
}

export function providerName(provider: string | null): string {
  if (provider === null) return "Not set"
  return providerNames[provider] ?? provider
}

export function effortName(effort: string | null): string {
  if (effort === null) return "Not used by this provider"
  return effortNames[effort] ?? effort.charAt(0).toUpperCase() + effort.slice(1)
}

function plural(count: number, unit: string): string {
  return `${count} ${unit}${count === 1 ? "" : "s"}`
}

export function sessionLength(seconds: number): string {
  if (seconds % 86400 === 0) return plural(seconds / 86400, "day")
  if (seconds % 3600 === 0) return plural(seconds / 3600, "hour")
  return plural(Math.round(seconds / 60), "minute")
}

export function domainsText(
  instance: Pick<Instance, "allowed_email_domains" | "any_email_domain">
): string {
  if (instance.any_email_domain) return "Any domain"
  return domainList(instance.allowed_email_domains).join(", ")
}
