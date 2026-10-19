import { formatShortRelative } from "@/lib/time"

import type { ServiceAccountKey } from "./queries"

export type KeyStatus = "active" | "expired" | "revoked"

export function keyStatus(
  key: Pick<ServiceAccountKey, "revoked_at" | "expires_at">,
  now = Date.now()
): KeyStatus {
  if (key.revoked_at !== null) return "revoked"
  if (key.expires_at !== null && Date.parse(key.expires_at) <= now) {
    return "expired"
  }
  return "active"
}

const dayMs = 86_400_000
const warningDays = 7
const distantDays = 730

export type ExpiryState = {
  tone: "ok" | "warning" | "critical"
  text: string
}

export function expiryState(
  key: Pick<ServiceAccountKey, "revoked_at" | "expires_at">,
  now = Date.now()
): ExpiryState {
  const status = keyStatus(key, now)
  if (status === "revoked") return { tone: "critical", text: "Revoked" }
  if (status === "expired") return { tone: "critical", text: "Expired" }
  if (key.expires_at === null) return { tone: "ok", text: "Never expires" }
  const left = Date.parse(key.expires_at) - now
  if (left < dayMs) {
    return { tone: "warning", text: formatShortRelative(key.expires_at, now) }
  }
  if (left >= distantDays * dayMs) {
    return { tone: "ok", text: formatShortRelative(key.expires_at, now) }
  }
  const days = Math.round(left / dayMs)
  return {
    tone: days < warningDays ? "warning" : "ok",
    text: days === 1 ? "Tomorrow" : `In ${days} days`,
  }
}

const statusRank: Record<KeyStatus, number> = {
  active: 0,
  expired: 1,
  revoked: 2,
}

export function sortKeys(
  keys: ServiceAccountKey[],
  now = Date.now()
): ServiceAccountKey[] {
  return keys.toSorted((a, b) => {
    const rank = statusRank[keyStatus(a, now)] - statusRank[keyStatus(b, now)]
    if (rank !== 0) return rank
    return Date.parse(b.created_at) - Date.parse(a.created_at)
  })
}

export const expiryOptions = [
  { value: "90d", label: "In 90 days", days: 90 },
  { value: "30d", label: "In 30 days", days: 30 },
  { value: "365d", label: "In 1 year", days: 365 },
  { value: "never", label: "Never", days: null },
] as const

export type ExpiryValue = (typeof expiryOptions)[number]["value"]

export const defaultExpiry: ExpiryValue = "90d"

export function expiresAtFor(
  value: ExpiryValue,
  now = Date.now()
): string | null {
  const days = expiryOptions.find((option) => option.value === value)?.days
  if (days === null || days === undefined) return null
  return new Date(now + days * 86_400_000).toISOString()
}

const accountNamePattern = /^[a-z0-9][a-z0-9-]{0,62}$/

export function keyNameError(name: string): string | null {
  const trimmed = name.trim()
  if (trimmed === "") return "Enter a key name."
  if ([...trimmed].length > 64) return "Use at most 64 characters."
  return null
}

export function accountNameError(name: string): string | null {
  if (name === "") return "Enter a name."
  if (!accountNamePattern.test(name)) {
    return "Use up to 63 lowercase letters, digits or dashes, starting with a letter or digit."
  }
  return null
}
