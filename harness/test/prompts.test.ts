import { describe, expect, test } from "bun:test"
import { createHash } from "node:crypto"
import { readFileSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"
import {
  buildSystem,
  compactJSON,
  investigateFor,
  type Variant,
} from "../src/prompts/run"
import { tasks, type Task } from "../src/route/route"

const goldens = resolve(import.meta.dir, "fixtures", "prompts")

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
const updating = process.env["UPDATE_GOLDENS"] === "1"
const guide = "guide-prometheus.md"

function variants(): Variant[] {
  return ["" as Task, ...tasks].flatMap((task) =>
    [false, true].flatMap((facts) =>
      [false, true].map((ledger) => ({ task, facts, ledger }))
    )
  )
}

if (updating) {
  const next: Cases = {
    investigate: variants().map((v) => ({
      ...v,
      guide,
      ...hashOf(investigateFor(v, golden(guide))),
    })),
    build: [{ guide, ...hashOf(buildSystem(golden(guide))) }],
  }
  writeFileSync(
    resolve(goldens, "cases.json"),
    JSON.stringify(next, null, 2) + "\n"
  )
  writeFileSync(
    resolve(goldens, "investigate-full-facts-ledger-prometheus.md"),
    investigateFor({ task: "", facts: true, ledger: true }, golden(guide))
  )
  writeFileSync(
    resolve(goldens, "build-prometheus.md"),
    buildSystem(golden(guide))
  )
}

describe("investigateFor", () => {
  test("matches its golden for every task and variant", () => {
    expect(cases.investigate.length).toBe(variants().length)
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
      investigateFor({ task: "", facts: true, ledger: true }, golden(guide))
    ).toBe(golden("investigate-full-facts-ledger-prometheus.md"))
  })
})

describe("buildSystem", () => {
  test("matches its golden", () => {
    expect(cases.build.length).toBe(1)
    for (const c of cases.build) {
      expect({
        guide: c.guide,
        ...hashOf(buildSystem(golden(c.guide))),
      }).toEqual({ guide: c.guide, bytes: c.bytes, sha256: c.sha256 })
    }
  })

  test("matches the readable golden", () => {
    expect(buildSystem(golden(guide))).toBe(golden("build-prometheus.md"))
  })
})

describe("compactJSON", () => {
  test("drops whitespace outside strings only", () => {
    expect(compactJSON('{ "a b": [1, 2],\n\t"c": "x \\" y" }')).toBe(
      '{"a b":[1,2],"c":"x \\" y"}'
    )
  })
})
