// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { rememberDatasource, rememberedDatasource } from "./datasource-memory"
import type { DatasourceStatus } from "./queries"

const storageKey = "slashobserve-explore-datasource"

function datasource(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "2026-11-06T00:00:00Z",
    dashboard_count: 0,
  }
}

const clickhouse = datasource("ch", "clickhouse")
const tempo = datasource("tempo", "tempo")
const loki = datasource("loki", "loki")

function stubStorage(initial: Record<string, string> = {}) {
  const store = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => void store.set(key, value),
    store,
  }
}

let storage: ReturnType<typeof stubStorage>

beforeEach(() => {
  storage = stubStorage()
  vi.stubGlobal("localStorage", storage)
})

afterEach(() => vi.unstubAllGlobals())

describe("rememberedDatasource", () => {
  it("keeps the stored datasource when it is still eligible", () => {
    storage.store.set(storageKey, JSON.stringify({ traces: "ch" }))
    expect(rememberedDatasource("traces", [tempo, clickhouse])).toBe("ch")
  })

  it("falls back when the stored datasource is gone", () => {
    storage.store.set(storageKey, JSON.stringify({ traces: "old" }))
    expect(rememberedDatasource("traces", [clickhouse, tempo])).toBe("tempo")
  })

  it("prefers Tempo for traces", () => {
    expect(rememberedDatasource("traces", [clickhouse, tempo])).toBe("tempo")
  })

  it("takes the first eligible for logs", () => {
    expect(rememberedDatasource("logs", [clickhouse, loki])).toBe("ch")
  })

  it("reads the page's own entry", () => {
    storage.store.set(storageKey, JSON.stringify({ logs: "ch" }))
    expect(rememberedDatasource("traces", [clickhouse, tempo])).toBe("tempo")
  })

  it("returns nothing without an eligible datasource", () => {
    expect(rememberedDatasource("traces", [])).toBeUndefined()
  })

  it.each(["", "not json", "[]", '{"traces": 3}'])(
    "ignores a stored value of %j",
    (raw) => {
      storage.store.set(storageKey, raw)
      expect(rememberedDatasource("traces", [clickhouse])).toBe("ch")
    }
  )

  it("survives storage that throws", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("blocked")
      },
      setItem: () => {
        throw new Error("blocked")
      },
    })
    expect(rememberedDatasource("traces", [clickhouse])).toBe("ch")
    expect(() => rememberDatasource("traces", "ch")).not.toThrow()
  })
})

describe("rememberDatasource", () => {
  it("keeps the other page's entry", () => {
    rememberDatasource("logs", "loki")
    rememberDatasource("traces", "tempo")
    expect(JSON.parse(storage.store.get(storageKey) ?? "")).toEqual({
      logs: "loki",
      traces: "tempo",
    })
  })
})
