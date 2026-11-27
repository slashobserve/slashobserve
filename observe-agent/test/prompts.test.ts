import { describe, expect, test } from "bun:test"
import { createHash } from "node:crypto"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import {
  buildSystem,
  compactJSON,
  investigateFor,
  type Variant,
} from "../src/prompts/run"
import { tasks, type Task } from "../src/route/route"

const goldens = resolve(
  import.meta.dir,
  "..",
  "..",
  "spec",
  "examples",
  "agent",
  "prompts"
)

function golden(name: string): string {
  return readFileSync(resolve(goldens, name), "utf8")
}

type Hash = { bytes: number; sha256: string }

type Cases = {
  investigate: (Hash & {
    task: Task
    facts: boolean
    ledger: boolean
    guide: string
  })[]
  build: (Hash & { guide: string })[]
}

function hashOf(text: string): Hash {
  return {
    bytes: Buffer.byteLength(text),
    sha256: createHash("sha256").update(text).digest("hex"),
  }
}

const cases = JSON.parse(golden("cases.json")) as Cases

describe("investigateFor", () => {
  test("is byte-identical to Go for every task, variant and guide", () => {
    expect(cases.investigate.length).toBe((tasks.length + 1) * 4 * 4)
    for (const c of cases.investigate) {
      const v: Variant = { task: c.task, facts: c.facts, ledger: c.ledger }
      expect({
        ...v,
        guide: c.guide,
        ...hashOf(investigateFor(v, golden(c.guide))),
      }).toEqual({ ...v, guide: c.guide, bytes: c.bytes, sha256: c.sha256 })
    }
  })

  test("matches the readable golden", () => {
    expect(
      investigateFor(
        { task: "", facts: true, ledger: true },
        golden("guide-prometheus.md")
      )
    ).toBe(golden("investigate-full-facts-ledger-prometheus.md"))
  })
})

describe("buildSystem", () => {
  test("is byte-identical to Go for every guide", () => {
    expect(cases.build.length).toBe(4)
    for (const c of cases.build) {
      expect({
        guide: c.guide,
        ...hashOf(buildSystem(golden(c.guide))),
      }).toEqual({ guide: c.guide, bytes: c.bytes, sha256: c.sha256 })
    }
  })

  test("matches the readable golden", () => {
    expect(buildSystem(golden("guide-prometheus.md"))).toBe(
      golden("build-prometheus.md")
    )
  })
})

describe("compactJSON", () => {
  test("drops whitespace outside strings only", () => {
    expect(compactJSON('{ "a b": [1, 2],\n\t"c": "x \\" y" }')).toBe(
      '{"a b":[1,2],"c":"x \\" y"}'
    )
  })
})
