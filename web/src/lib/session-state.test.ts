import { describe, expect, it } from "vitest"

import type { Principal, Session } from "./auth"
import {
  agreesWith,
  reduceSession,
  unknownSession,
  type SessionEvent,
  type SessionState,
} from "./session-state"

const ana: Principal = { kind: "user", id: "u1", name: "Ana", role: "editor" }
const signedIn: Session = { authenticated: true, principal: ana }
const signedOut: Session = { authenticated: false }

const started: SessionEvent = { type: "check_started" }
const failed: SessionEvent = { type: "check_failed" }
const sawSignedIn: SessionEvent = { type: "answered", session: signedIn }
const sawSignedOut: SessionEvent = { type: "answered", session: signedOut }
const left: SessionEvent = { type: "left" }

function walk(...events: SessionEvent[]): SessionState {
  return events.reduce(reduceSession, unknownSession)
}

describe("reduceSession", () => {
  it("checks before it knows anything", () => {
    expect(walk(started)).toEqual({ status: "checking" })
    expect(walk(started, failed)).toEqual({ status: "unknown" })
  })

  it("is signed out for a visitor who never had a session", () => {
    expect(walk(started, sawSignedOut)).toEqual({ status: "signed_out" })
    expect(walk(sawSignedOut, sawSignedOut)).toEqual({ status: "signed_out" })
  })

  it("is signed in with the principal the server names", () => {
    expect(walk(started, sawSignedIn)).toEqual({
      status: "signed_in",
      principal: ana,
    })
  })

  it("keeps what it knows through a later check", () => {
    expect(walk(sawSignedIn, started)).toEqual(walk(sawSignedIn))
    expect(walk(sawSignedIn, started, failed)).toEqual(walk(sawSignedIn))
  })

  it("has ended when a known session is gone without the person leaving", () => {
    expect(walk(sawSignedIn, sawSignedOut)).toEqual({ status: "ended" })
    expect(walk(sawSignedIn, sawSignedOut, sawSignedOut)).toEqual({
      status: "ended",
    })
  })

  it("is signed out, not ended, after the person leaves on purpose", () => {
    expect(walk(sawSignedIn, left)).toEqual({ status: "signed_out" })
    expect(walk(sawSignedIn, left, sawSignedOut)).toEqual({
      status: "signed_out",
    })
  })

  it("clears ended on the next sign-in or on leaving", () => {
    expect(walk(sawSignedIn, sawSignedOut, sawSignedIn).status).toBe(
      "signed_in"
    )
    expect(walk(sawSignedIn, sawSignedOut, left)).toEqual({
      status: "signed_out",
    })
  })
})

describe("agreesWith", () => {
  it("is false until a session is known", () => {
    expect(agreesWith(walk(), signedOut)).toBe(false)
    expect(agreesWith(walk(started), signedIn)).toBe(false)
  })

  it("compares the principal's id and role", () => {
    expect(agreesWith(walk(sawSignedIn), signedIn)).toBe(true)
    expect(
      agreesWith(walk(sawSignedIn), {
        authenticated: true,
        principal: { ...ana, role: "admin" },
      })
    ).toBe(false)
    expect(agreesWith(walk(sawSignedIn), signedOut)).toBe(false)
  })

  it("treats signed out and ended alike", () => {
    expect(agreesWith(walk(sawSignedOut), signedOut)).toBe(true)
    expect(agreesWith(walk(sawSignedIn, sawSignedOut), signedOut)).toBe(true)
  })
})
