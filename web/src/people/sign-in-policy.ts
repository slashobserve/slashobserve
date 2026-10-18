import type { Instance } from "@/api/instance"

const onlineWindowMs = 2 * 60_000

export function providerLabel(name: string | undefined): string {
  const trimmed = name?.trim() ?? ""
  return trimmed === "" ? "your identity provider" : trimmed
}

export function domainList(domains: string[]): string[] {
  const seen = new Set<string>()
  for (const domain of domains) {
    const trimmed = domain.trim()
    if (trimmed !== "") seen.add(trimmed)
  }
  return [...seen]
}

export function isAllowAll(instance: Pick<Instance, "any_email_domain">) {
  return instance.any_email_domain
}

export function domainSeparator(index: number, count: number): string {
  if (index === 0) return ""
  return index === count - 1 ? " or " : ", "
}

export function joinDomains(domains: string[]): string {
  return domains
    .map((domain, index) => domainSeparator(index, domains.length) + domain)
    .join("")
}

export function isOnline(lastSeenAt: string | null, now: number): boolean {
  if (lastSeenAt === null) return false
  const seen = Date.parse(lastSeenAt)
  return !Number.isNaN(seen) && now - seen <= onlineWindowMs
}
