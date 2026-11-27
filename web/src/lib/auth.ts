import type { Role } from "./roles"

export type Principal = {
  kind: "user" | "service_account"
  id: string
  email?: string
  name: string
  role: Role
}

export type Session =
  { authenticated: false } | { authenticated: true; principal: Principal }

let cachedSession: Promise<Session> | undefined
let knownSession: Session | undefined
let generation = 0
let ended = false

function remember(session: Session) {
  if (knownSession?.authenticated && !session.authenticated) ended = true
  if (session.authenticated) ended = false
  knownSession = session
}

export function signedInPrincipalId(): string | undefined {
  return knownSession?.authenticated ? knownSession.principal.id : undefined
}

export function sessionEnded(): boolean {
  return ended
}

export function getSession(): Promise<Session> {
  if (cachedSession) return cachedSession
  const current = ++generation
  const pending: Promise<Session> = fetchSession().then(
    (session) => {
      if (generation === current) remember(session)
      return session
    },
    (error: unknown) => {
      if (cachedSession === pending) cachedSession = undefined
      throw error
    }
  )
  cachedSession = pending
  return pending
}

export async function refreshSession(): Promise<{
  session: Session
  changed: boolean
}> {
  const current = ++generation
  const session = await fetchSession()
  if (generation !== current) return { session, changed: false }
  const changed = !sameSession(knownSession, session)
  remember(session)
  cachedSession = Promise.resolve(session)
  return { session, changed }
}

export function invalidateSession(): void {
  generation++
  cachedSession = undefined
}

function sameSession(a: Session | undefined, b: Session): boolean {
  if (!a || a.authenticated !== b.authenticated) return false
  if (!a.authenticated || !b.authenticated) return true
  return (
    a.principal.id === b.principal.id && a.principal.role === b.principal.role
  )
}

async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
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
  knownSession = { authenticated: false }
}

export function loginHref(returnTo: string): string {
  return `/api/v1/auth/login?return_to=${encodeURIComponent(returnTo)}`
}
