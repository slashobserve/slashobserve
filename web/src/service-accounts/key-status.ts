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

export const expiryOptions = [
  { value: "30d", label: "30 days", days: 30 },
  { value: "90d", label: "90 days", days: 90 },
  { value: "365d", label: "1 year", days: 365 },
  { value: "never", label: "No expiry", days: null },
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

export function descriptionError(text: string): string | null {
  if ([...text].length > 256) return "Use at most 256 characters."
  return null
}
