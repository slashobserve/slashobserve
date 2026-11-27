import type { Role } from "./roles"
import {
  agreesWith,
  reduceSession,
  unknownSession,
  type SessionEvent,
} from "./session-state"

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
let generation = 0
let state = unknownSession

function move(event: SessionEvent) {
  state = reduceSession(state, event)
}

export function signedInPrincipalId(): string | undefined {
  return state.status === "signed_in" ? state.principal.id : undefined
}

export function sessionEnded(): boolean {
  return state.status === "ended"
}

export function getSession(): Promise<Session> {
  if (cachedSession) return cachedSession
  const current = ++generation
  move({ type: "check_started" })
  const pending: Promise<Session> = fetchSession().then(
    (session) => {
      if (generation === current) move({ type: "answered", session })
      return session
    },
    (error: unknown) => {
      if (cachedSession === pending) cachedSession = undefined
      move({ type: "check_failed" })
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
  const changed = !agreesWith(state, session)
  move({ type: "answered", session })
  cachedSession = Promise.resolve(session)
  return { session, changed }
}

export function invalidateSession(): void {
  generation++
  cachedSession = undefined
}

export function leaveSession(): void {
  move({ type: "left" })
  invalidateSession()
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
  move({ type: "left" })
}

export function loginHref(returnTo: string): string {
  return `/api/v1/auth/login?return_to=${encodeURIComponent(returnTo)}`
}
