import type { Principal, Session } from "./auth"

export type SessionState =
  | { status: "unknown" }
  | { status: "checking" }
  | { status: "signed_in"; principal: Principal }
  | { status: "signed_out" }
  | { status: "ended" }

export type SessionEvent =
  | { type: "check_started" }
  | { type: "check_failed" }
  | { type: "answered"; session: Session }
  | { type: "left" }

export const unknownSession: SessionState = { status: "unknown" }

export function reduceSession(
  state: SessionState,
  event: SessionEvent
): SessionState {
  switch (event.type) {
    case "check_started":
      return state.status === "unknown" ? { status: "checking" } : state
    case "check_failed":
      return state.status === "checking" ? unknownSession : state
    case "answered":
      if (event.session.authenticated) {
        return { status: "signed_in", principal: event.session.principal }
      }
      return state.status === "signed_in" || state.status === "ended"
        ? { status: "ended" }
        : { status: "signed_out" }
    case "left":
      return { status: "signed_out" }
  }
}

export function agreesWith(state: SessionState, session: Session): boolean {
  if (!session.authenticated) {
    return state.status === "signed_out" || state.status === "ended"
  }
  return (
    state.status === "signed_in" &&
    state.principal.id === session.principal.id &&
    state.principal.role === session.principal.role
  )
}
