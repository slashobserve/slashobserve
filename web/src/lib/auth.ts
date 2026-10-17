export type Principal = {
  kind: "user" | "service_account"
  id: string
  email?: string
  name: string
}

export type Session =
  { authenticated: false } | { authenticated: true; principal: Principal }

export async function getSession(): Promise<Session> {
  const response = await fetch("/api/v1/auth/session", {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  })
  if (!response.ok) throw new Error(`session check failed: ${response.status}`)
  return (await response.json()) as Session
}

export async function logout(): Promise<void> {
  const response = await fetch("/api/v1/auth/logout", {
    method: "POST",
    credentials: "same-origin",
  })
  if (!response.ok) throw new Error(`sign out failed: ${response.status}`)
}

export function loginHref(returnTo: string): string {
  return `/api/v1/auth/login?return_to=${encodeURIComponent(returnTo)}`
}
