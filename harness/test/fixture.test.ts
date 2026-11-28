import { describe, expect, test } from "bun:test"
import {
  FixtureError,
  parseFixture,
  promptKey,
  turnsFor,
} from "../src/session/replay/fixture"

function failure(content: string): string {
  try {
    parseFixture("f.json", content)
  } catch (error) {
    expect(error).toBeInstanceOf(FixtureError)
    return (error as Error).message
  }
  throw new Error("parseFixture accepted the content")
}

function cases(
  rows: Array<[name: string, content: string, wantErr: string]>
): void {
  for (const [name, content, wantErr] of rows) {
    test(name, () => {
      if (wantErr === "") {
        expect(() => parseFixture("f.json", content)).not.toThrow()
        return
      }
      expect(failure(content)).toContain(wantErr)
    })
  }
}

describe("fixture turns", () => {
  test("allows turns after a turn without tool calls", () => {
    expect(() =>
      parseFixture(
        "f.json",
        `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"text":"done"},{"tool_calls":[{"name":"t"}]}]}`
      )
    ).not.toThrow()
  })

  test("allows tool calls on the last turn", () => {
    const content =
      `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"tool_calls":[{"name":"t"}],"expect":[{"is_error":false}]},` +
      `{"tool_calls":[{"name":"t"}],"expect":[{"is_error":false}]}]}`
    expect(() => parseFixture("f.json", content)).not.toThrow()
  })

  test("keeps a tool call input as compact JSON text", () => {
    const fixture = parseFixture(
      "f.json",
      `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":{ "a" : [1, 2] }},{"name":"u"}]}]}`
    )
    expect(fixture.turns[0]?.toolCalls).toEqual([
      { name: "t", input: `{"a":[1,2]}` },
      { name: "u", input: "{}" },
    ])
  })
})

describe("fixture fault validation", () => {
  cases([
    [
      "fault without text or calls",
      `{"prompt":"p","turns":[{"fault":{"kind":"refusal"}}]}`,
      "",
    ],
    [
      "delay with a duration",
      `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":5},"text":"ok"}]}`,
      "",
    ],
    [
      "unknown kind",
      `{"prompt":"p","turns":[{"fault":{"kind":"meltdown"},"text":"x"}]}`,
      `kind "meltdown" is not one of`,
    ],
    [
      "missing kind",
      `{"prompt":"p","turns":[{"fault":{},"text":"x"}]}`,
      "is not one of",
    ],
    [
      "negative delay",
      `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":-1}}]}`,
      "delay_ms must not be negative",
    ],
    [
      "unknown fault field",
      `{"prompt":"p","turns":[{"fault":{"kind":"delay","ms":1}}]}`,
      "unknown field",
    ],
    [
      "turn without fault still needs content",
      `{"prompt":"p","turns":[{}]}`,
      "needs text or tool_calls",
    ],
  ])
})

const resumeTurns = `"turns":[{"tool_calls":[{"name":"t"}]}]`

describe("fixture resume segment validation", () => {
  cases([
    [
      "valid segments",
      `{"prompt":"p",${resumeTurns},"resumes":{"1":{"turns":[{"text":"a"}]},"12":{"turns":[{"text":"b"}]}}}`,
      "",
    ],
    [
      "zero key",
      `{"prompt":"p",${resumeTurns},"resumes":{"0":{"turns":[{"text":"a"}]}}}`,
      `key "0" is not a positive integer`,
    ],
    [
      "negative key",
      `{"prompt":"p",${resumeTurns},"resumes":{"-1":{"turns":[{"text":"a"}]}}}`,
      `key "-1" is not a positive integer`,
    ],
    [
      "padded key",
      `{"prompt":"p",${resumeTurns},"resumes":{"01":{"turns":[{"text":"a"}]}}}`,
      `key "01" is not a positive integer`,
    ],
    [
      "word key",
      `{"prompt":"p",${resumeTurns},"resumes":{"one":{"turns":[{"text":"a"}]}}}`,
      `key "one" is not a positive integer`,
    ],
    [
      "empty segment",
      `{"prompt":"p",${resumeTurns},"resumes":{"1":{"turns":[]}}}`,
      `resumes."1".turns needs at least one turn`,
    ],
    [
      "turn after a text turn in segment",
      `{"prompt":"p",${resumeTurns},"resumes":{"2":{"turns":[{"text":"a"},{"text":"b"}]}}}`,
      "",
    ],
    [
      "empty turn in segment",
      `{"prompt":"p",${resumeTurns},"resumes":{"1":{"turns":[{}]}}}`,
      "needs text or tool_calls",
    ],
    [
      "unknown segment field",
      `{"prompt":"p",${resumeTurns},"resumes":{"1":{"turns":[{"text":"a"}],"extra":1}}}`,
      "unknown field",
    ],
    [
      "turns stay required",
      `{"prompt":"p","resumes":{"1":{"turns":[{"text":"a"}]}}}`,
      "turns needs at least one turn",
    ],
  ])
})

const saveAfterFixture =
  `{"prompt":"p","resume_after_saves":true,"turns":[` +
  `{"tool_calls":[{"name":"create_dashboard"}]},` +
  `{"tool_calls":[{"name":"list_metrics"}]},` +
  `{"tool_calls":[{"name":"update_panels"}]},` +
  `{"tool_calls":[{"name":"update_panels"}]}]}`

describe("fixture resume after saves", () => {
  const fixture = parseFixture("f.json", saveAfterFixture)
  const rows: Array<{
    name: string
    saves: number
    first: string
    turns: number
    found: boolean
  }> = [
    {
      name: "a fresh session reads every turn",
      saves: 0,
      first: "create_dashboard",
      turns: 4,
      found: true,
    },
    {
      name: "after the first save the read turn follows",
      saves: 1,
      first: "list_metrics",
      turns: 3,
      found: true,
    },
    {
      name: "a read turn between saves is not counted",
      saves: 2,
      first: "update_panels",
      turns: 1,
      found: true,
    },
    {
      name: "after the last save nothing is left",
      saves: 3,
      first: "",
      turns: 0,
      found: true,
    },
    {
      name: "past the last save there is no fixture",
      saves: 4,
      first: "",
      turns: 0,
      found: false,
    },
  ]
  for (const row of rows) {
    test(row.name, () => {
      const turns = turnsFor(fixture, row.saves)
      expect(turns !== undefined).toBe(row.found)
      expect(turns?.length ?? 0).toBe(row.turns)
      if (row.first !== "") {
        expect(turns?.[0]?.toolCalls[0]?.name).toBe(row.first)
      }
    })
  }

  describe("validation", () => {
    cases([
      ["valid", saveAfterFixture, ""],
      [
        "with resumes",
        `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"update_panels"}]}],"resumes":{"1":{"turns":[{"text":"a"}]}}}`,
        "cannot be combined with resumes",
      ],
      [
        "two saves in one turn",
        `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"update_panels"},{"name":"update_panels"}]}]}`,
        "turn 0 has 2 save calls",
      ],
      [
        "a read and a save in one turn",
        `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"lint_query"},{"name":"update_panels"}]}]}`,
        "",
      ],
      [
        "expect after a save",
        `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"update_panels"}]},{"tool_calls":[{"name":"lint_query"}],"expect":[{"is_error":false}]}]}`,
        "expect is not allowed on the turn after a save",
      ],
      [
        "expect after a read",
        `{"prompt":"p","resume_after_saves":true,"turns":[{"tool_calls":[{"name":"lint_query"}]},{"tool_calls":[{"name":"lint_query"}],"expect":[{"is_error":false}]}]}`,
        "",
      ],
      [
        "two saves are fine without the flag",
        `{"prompt":"p","turns":[{"tool_calls":[{"name":"update_panels"},{"name":"update_panels"}]}]}`,
        "",
      ],
    ])
  })
})

describe("fixture shape errors name the path", () => {
  const turn = `{"text":"a"}`
  const table: Array<[string, string]> = [
    ["", "not valid JSON"],
    [`{"prompt":"p","turns":[${turn}]} x`, "not valid JSON"],
    ["[]", "must be an object"],
    [`{"Prompt":"p","turns":[${turn}]}`, `unknown field "Prompt"`],
    [`{"prompt":5,"turns":[${turn}]}`, "prompt: must be a string"],
    [`{"prompt":null,"turns":[${turn}]}`, "prompt: must be a string"],
    [`{"prompt":"p","turns":"x"}`, "turns: must be an array"],
    [`{"prompt":"p","turns":[1]}`, "turns[0]: must be an object"],
    [
      `{"prompt":"p","turns":[{"tool_calls":[{"name":1}]}]}`,
      "turns[0].tool_calls[0].name: must be a string",
    ],
    [
      `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":"x"}]}]}`,
      "turns[0].tool_calls[0].input: must be an object",
    ],
    [
      `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"text":"a","expect":[{"is_error":"no"}]}]}`,
      "turns[1].expect[0].is_error: must be true or false",
    ],
    [
      `{"prompt":"p","turns":[{"text":"a","fault":{"kind":"delay","delay_ms":1.5}}]}`,
      "turns[0].fault.delay_ms: must be a whole number",
    ],
    [
      `{"prompt":"p","turns":[${turn}],"resume_after_saves":"yes"}`,
      "resume_after_saves: must be true or false",
    ],
    [
      `{"prompt":"p","turns":[${turn}],"resumes":[]}`,
      "resumes: must be an object",
    ],
  ]
  for (const [content, message] of table) {
    test(JSON.stringify(content).slice(0, 80), () => {
      const got = failure(content)
      expect(got.startsWith("replay fixture f.json: ")).toBe(true)
      expect(got).toContain(message)
    })
  }
})

describe("promptKey", () => {
  const rows: Array<[string, string]> = [
    ["  Mixed\tCASE\n", "mixed case"],
    ["Show   checkout HEALTH", "show checkout health"],
    ["a b\u3000c d", "a b c d"],
    ["", ""],
  ]
  for (const [prompt, key] of rows) {
    test(JSON.stringify(prompt), () => {
      expect(promptKey(prompt)).toBe(key)
    })
  }
})
