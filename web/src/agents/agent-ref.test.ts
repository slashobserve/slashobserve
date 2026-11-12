import { describe, expect, test } from "vitest"

import {
  agentRefPath,
  findAgent,
  parseAgentRef,
  type AgentRef,
} from "./agent-ref"
import type { AgentsOverview } from "./queries"

const stats = {
  runs: 1,
  failed: 0,
  cost_usd: null,
  median_queue_wait_seconds: null,
}

const overview: AgentsOverview = {
  observe_agent: {
    enabled: true,
    provider: null,
    model: null,
    runtime: "go",
    scope: "you",
    last_7_days: stats,
  },
  connected: [
    {
      grant_id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
      client_name: "Claude Code",
      mode: "direct",
      present: false,
      last_seen_at: null,
      connected_at: "2026-11-11T10:00:00Z",
      writes_7d: 0,
      last_7_days: stats,
    },
  ],
  service_accounts: [
    {
      id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
      name: "nightly-audit",
      present: true,
      last_seen_at: null,
      last_7_days: null,
    },
  ],
}

describe("agent references", () => {
  test.each<[AgentRef, string]>([
    [{ kind: "observe" }, "observe"],
    [
      { kind: "grant", id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01" },
      "grant-0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
    ],
    [
      { kind: "service_account", id: "5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11" },
      "sa-5f0a8e3c-2b7d-4e0b-a1c4-7d6f0e9b2c11",
    ],
  ])("%j round trips through %s", (ref, path) => {
    expect(agentRefPath(ref)).toBe(path)
    expect(parseAgentRef(path)).toEqual(ref)
  })

  test.each([
    "",
    "Observe",
    "observe-1",
    "grant-",
    "sa-",
    "grant-a/b",
    "grant-a b",
    "user-123",
    `grant-${"a".repeat(65)}`,
  ])("%j is not a reference", (text) => {
    expect(parseAgentRef(text)).toBeNull()
  })
})

describe("findAgent", () => {
  test("finds each kind of row", () => {
    expect(findAgent(overview, { kind: "observe" })).toEqual({
      kind: "observe",
      row: overview.observe_agent,
    })
    expect(
      findAgent(overview, {
        kind: "grant",
        id: overview.connected[0]!.grant_id,
      })
    ).toEqual({ kind: "grant", row: overview.connected[0] })
    expect(
      findAgent(overview, {
        kind: "service_account",
        id: overview.service_accounts[0]!.id,
      })
    ).toEqual({ kind: "service_account", row: overview.service_accounts[0] })
  })

  test("misses a grant or account that is not listed", () => {
    expect(findAgent(overview, { kind: "grant", id: "gone" })).toBeNull()
    expect(
      findAgent(overview, { kind: "service_account", id: "gone" })
    ).toBeNull()
  })

  test("does not mix up a grant id with an account id", () => {
    expect(
      findAgent(overview, {
        kind: "service_account",
        id: overview.connected[0]!.grant_id,
      })
    ).toBeNull()
  })
})
