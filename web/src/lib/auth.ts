export type Principal = {
  kind: "user" | "service_account"
  id: string
  email?: string
  name: string
}

export type Session =
  { authenticated: false } | { authenticated: true; principal: Principal }

let cachedSession: Promise<Session> | undefined

export function getSession(): Promise<Session> {
  cachedSession ??= fetchSession().catch((error: unknown) => {
    cachedSession = undefined
    throw error
  })
  return cachedSession
}

export async function refreshSession(): Promise<Session> {
  const session = await fetchSession()
  cachedSession = Promise.resolve(session)
  return session
}

export function invalidateSession(): void {
  cachedSession = undefined
}

export async function apiFetch(
  input: string,
  init?: RequestInit
): Promise<Response> {
  const response = await fetch(input, { credentials: "same-origin", ...init })
  if (response.status === 401) invalidateSession()
  return response
}

async function fetchSession(): Promise<Session> {
  const response = await apiFetch("/api/v1/auth/session", {
    headers: { Accept: "application/json" },
  })
  if (!response.ok) throw new Error(`session check failed: ${response.status}`)
  return (await response.json()) as Session
}

export async function logout(): Promise<void> {
  const response = await apiFetch("/api/v1/auth/logout", { method: "POST" })
  if (!response.ok) throw new Error(`sign out failed: ${response.status}`)
}

export function loginHref(returnTo: string): string {
  return `/api/v1/auth/login?return_to=${encodeURIComponent(returnTo)}`
}
