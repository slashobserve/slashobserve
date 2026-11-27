import { describe, expect, it } from "vitest"

import {
  accountWithoutKey,
  emptyKeyPanel,
  reduceKeyPanel,
  type KeyPanelEvent,
  type KeyPanelState,
} from "./create-key-state"
import type { ServiceAccount } from "./queries"

const created: ServiceAccount = {
  id: "a2",
  name: "new-bot",
  description: "",
  created_by: "admin-1",
  created_at: "2026-11-01T00:00:00Z",
  role: "editor",
  provisioned: false,
  takes_runs: false,
  presence: "offline",
  last_seen_at: null,
}

function walk(...events: KeyPanelEvent[]): KeyPanelState {
  return events.reduce(reduceKeyPanel, emptyKeyPanel)
}

const started: KeyPanelEvent = { type: "submit_started" }
const accountCreated: KeyPanelEvent = {
  type: "account_created",
  account: created,
}
const keyFailed: KeyPanelEvent = { type: "failed", problem: "No key." }
const issued: KeyPanelEvent = {
  type: "key_issued",
  token: "sok_test",
  accountName: "new-bot",
  role: "editor",
}

describe("reduceKeyPanel", () => {
  it("starts on an empty form", () => {
    expect(walk().step).toEqual({ phase: "form", problem: null })
  })

  it("resets the role and the switch with the account", () => {
    const state = walk(
      { type: "role_chosen", role: "viewer" },
      { type: "takes_runs_chosen", takesRuns: true },
      { type: "account_chosen", choice: "a1" }
    )
    expect(state.accountChoice).toBe("a1")
    expect(state.roleChoice).toBeNull()
    expect(state.takesRunsChoice).toBeNull()
  })

  it("marks a refused submit without leaving the form", () => {
    const state = walk({ type: "submit_refused" })
    expect(state.submitted).toBe(true)
    expect(state.step.phase).toBe("form")
  })

  it("clears the last problem when a submit starts", () => {
    const state = walk(started, keyFailed, started)
    expect(state.step).toEqual({ phase: "creating" })
  })

  it("ignores field edits while creating", () => {
    const state = walk(started, { type: "key_named", name: "late" })
    expect(state.keyName).toBe("")
  })

  it("returns to the form when the account itself was not created", () => {
    const state = walk(started, keyFailed)
    expect(state.step).toEqual({ phase: "form", problem: "No key." })
    expect(accountWithoutKey(state)).toBeNull()
  })

  it("is partly done when the key fails after the account was created", () => {
    const state = walk(started, accountCreated, keyFailed)
    expect(state.step).toEqual({ phase: "partly_done", problem: "No key." })
    expect(state.accountChoice).toBe("a2")
    expect(accountWithoutKey(state)).toEqual(created)
  })

  it("is partly done when the new account's update fails", () => {
    const state = walk(started, accountCreated, {
      type: "failed",
      problem: "No update.",
    })
    expect(state.step.phase).toBe("partly_done")
  })

  it("stays partly done through a second failed key", () => {
    const state = walk(started, accountCreated, keyFailed, started, keyFailed)
    expect(accountWithoutKey(state)).toEqual(created)
  })

  it("issues the key for the created account on the next submit", () => {
    const state = walk(started, accountCreated, keyFailed, started, issued)
    expect(state.step).toEqual({
      phase: "issued",
      token: "sok_test",
      accountName: "new-bot",
      role: "editor",
    })
    expect(accountWithoutKey(state)).toBeNull()
  })

  it("leaves partly done when another account is picked", () => {
    const state = walk(started, accountCreated, keyFailed, {
      type: "account_chosen",
      choice: "a1",
    })
    expect(state.step).toEqual({ phase: "form", problem: "No key." })
    expect(state.createdAccount).toEqual(created)
    const failedElsewhere = reduceKeyPanel(
      reduceKeyPanel(state, started),
      keyFailed
    )
    expect(failedElsewhere.step.phase).toBe("form")
  })

  it("keeps the created account's latest copy", () => {
    const updated = { ...created, takes_runs: true }
    const state = walk(started, accountCreated, {
      type: "account_updated",
      account: updated,
    })
    expect(state.createdAccount).toEqual(updated)
  })

  it("never leaves issued", () => {
    const state = walk(started, issued, started, keyFailed, {
      type: "key_named",
      name: "again",
    })
    expect(state.step.phase).toBe("issued")
    expect(state.keyName).toBe("")
  })
})
