import { describe, expect, test } from "bun:test"
import { createHash } from "node:crypto"
import { readdirSync, readFileSync } from "node:fs"
import { resolve } from "node:path"
import { describeAnchor, type Anchor } from "../src/brief/anchor"
import { addDiscovery, digestText, type Discovery } from "../src/brief/digest"
import { goJSON, goQuote } from "../src/brief/gofmt"
import { render, type BriefState } from "../src/brief/render"
import { resumeReason, type Reason } from "../src/brief/seed"
import { runBrief } from "../src/protocol"

const examples = resolve(import.meta.dir, "..", "..", "spec", "examples")

function read(...path: string[]): string {
  return readFileSync(resolve(examples, ...path), "utf8")
}

type Golden = {
  name: string
  brief: unknown
  guide: string
  reason: Reason
  now: string
  state: BriefState & {
    ledger: BriefState["ledger"] | null
    digest: Discovery[] | null
  }
  want: {
    system: { bytes: number; sha256: string }
    context: string
    user: string
  }
}

const goldenDir = resolve(examples, "agent", "brief")
const goldens = readdirSync(goldenDir)
  .filter((name) => name.endsWith(".json"))
  .map((name) => JSON.parse(read("agent", "brief", name)) as Golden)

describe("render", () => {
  test("covers every run shape the goldens name", () => {
    expect(goldens.map((g) => g.name).sort()).toEqual([
      "block-ask",
      "continue-build",
      "continue-investigate",
      "create-build-renewed",
      "create-fresh",
      "investigate-resume-after-compaction",
      "investigate-triage-ledger-facts",
      "revise-build-restart",
      "revise-missing-panel",
      "revise-outline",
      "suggest-draft",
      "thread-ask-anchor",
      "thread-ask-long-quote",
      "thread-ask-page",
    ])
  })

  for (const g of goldens) {
    test(`writes Go's brief for ${g.name}`, () => {
      const brief = runBrief.parse(g.brief)
      brief.guide.text = read("agent", "prompts", g.guide)
      const state: BriefState = {
        ...g.state,
        ledger: g.state.ledger ?? [],
        digest: g.state.digest ?? [],
      }
      const got = render(brief, g.reason, state, new Date(g.now))
      expect(got.context).toBe(g.want.context)
      expect(got.user).toBe(g.want.user)
      expect({
        bytes: Buffer.byteLength(got.system),
        sha256: createHash("sha256").update(got.system).digest("hex"),
      }).toEqual(g.want.system)
    })
  }
})

describe("describeAnchor", () => {
  type AnchorCase = { name: string; anchor: Anchor; describe: string }
  const cases = (
    JSON.parse(read("anchor", "anchor-cases.json")) as { valid: AnchorCase[] }
  ).valid

  test("names what each anchor points at as Go does", () => {
    expect(cases.length).toBeGreaterThan(0)
    for (const c of cases) {
      expect({ name: c.name, text: describeAnchor(c.anchor) }).toEqual({
        name: c.name,
        text: c.describe,
      })
    }
  })
})

describe("the digest", () => {
  test("keeps one entry per datasource and kind, newest last", () => {
    let entries: Discovery[] = []
    entries = addDiscovery(entries, {
      datasource: "p",
      of: "metrics",
      names: ["a"],
    })
    entries = addDiscovery(entries, {
      datasource: "p",
      of: "labels",
      names: ["l"],
    })
    entries = addDiscovery(entries, {
      datasource: "p",
      of: "metrics",
      names: ["b"],
    })
    entries = addDiscovery(entries, {
      datasource: "p",
      of: "labels",
      names: [],
    })
    expect(entries).toEqual([
      { datasource: "p", of: "labels", names: ["l"] },
      { datasource: "p", of: "metrics", names: ["b"] },
    ])
    expect(digestText(entries)).toBe(
      "<discovered>\nNames this run already found; use them without listing them again.\n- p metrics: b\n- p labels: l\n</discovered>"
    )
  })

  test("stays within 4 KiB and drops the oldest entries", () => {
    const names = Array.from({ length: 600 }, (_, i) => `metric_${i}`)
    const entries = [
      { datasource: "old", of: "metrics", names: ["kept_out"] },
      { datasource: "new", of: "metrics", names },
    ]
    expect(digestText(entries)).toBe("")
    const small = [
      { datasource: "old", of: "metrics", names: names.slice(0, 200) },
      { datasource: "new", of: "metrics", names: names.slice(0, 150) },
    ]
    const text = digestText(small)
    expect(text).toContain("- new metrics:")
    expect(text).not.toContain("- old metrics:")
    expect(Buffer.byteLength(text)).toBeLessThanOrEqual(4096)
  })

  test("is empty without entries", () => {
    expect(digestText([])).toBe("")
  })
})

describe("resumeReason", () => {
  test("continues only on the first attempt of a continuation", () => {
    expect(resumeReason({ continues: true, attempt: 1 })).toBe("continue")
    expect(resumeReason({ continues: true, attempt: 2 })).toBe("restart")
    expect(resumeReason({ continues: false, attempt: 1 })).toBe("restart")
  })
})

describe("Go formatting", () => {
  test("quotes like strconv.Quote", () => {
    expect(goQuote('a "b" \\ é\n\t\u0007\u007f​\u{1f600}\u{e0001}')).toBe(
      '"a \\"b\\" \\\\ é\\n\\t\\a\\x7f\\u200b\u{1f600}\\U000e0001"'
    )
  })

  test("escapes HTML in JSON like encoding/json", () => {
    expect(goJSON({ a: "<b>& ", n: 2.5e-7, m: 1e21 })).toBe(
      '{"a":"\\u003cb\\u003e\\u0026\\u2028","n":2.5e-7,"m":1e+21}'
    )
  })
})
