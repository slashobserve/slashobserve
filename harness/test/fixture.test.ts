import { describe, expect, test } from "bun:test"
import { goQuote } from "../src/session/replay/goquote"
import {
  FixtureError,
  parseFixture,
  promptKey,
  turnsFor,
} from "../src/session/replay/fixture"
import { parseJsonDocument } from "../src/session/replay/raw"

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

  test("keeps a tool call input as its raw JSON text", () => {
    const fixture = parseFixture(
      "f.json",
      `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":{ "a" : [1, 2] }},{"name":"u"}]}]}`
    )
    expect(fixture.turns[0]?.toolCalls).toEqual([
      { name: "t", input: `{ "a" : [1, 2] }` },
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

describe("fixture error texts match the Go decoder", () => {
  const table: Array<[string, string]> = [
    ["", "EOF"],
    ["   ", "EOF"],
    ["[]", "json: cannot unmarshal array into Go value of type replay.fixture"],
    [
      "123",
      "json: cannot unmarshal number into Go value of type replay.fixture",
    ],
    ['{"prompt"', "unexpected EOF"],
    [
      '{"prompt":"p",}',
      "invalid character '}' looking for beginning of object key string",
    ],
    [
      '{"prompt":"p" "turns":[]}',
      "invalid character '\"' after object key:value pair",
    ],
    ['{"prompt" "p"}', "invalid character '\"' after object key"],
    [
      '{"prompt":"p","turns":[{"text":"a"}]} x',
      "unexpected content after the fixture object",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a"}]} {}',
      "unexpected content after the fixture object",
    ],
    [
      '{"prompt":5,"turns":[{"text":"a"}]}',
      "json: cannot unmarshal number into Go struct field fixture.prompt of type string",
    ],
    [
      '{"prompt":"p","turns":"x"}',
      "json: cannot unmarshal string into Go struct field fixture.turns of type []replay.turn",
    ],
    [
      '{"prompt":"p","turns":[1]}',
      "json: cannot unmarshal number into fixture.turns.0 of type replay.turn",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","tool_calls":[{"name":5}]}]}',
      "json: cannot unmarshal number into Go struct field fixture.turns.0.tool_calls.0.name of type string",
    ],
    [
      '{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":1.5}}]}',
      "json: cannot unmarshal number 1.5 into Go struct field fixture.turns.0.fault.delay_ms of type int",
    ],
    [
      '{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":99999999999999999999}}]}',
      "json: cannot unmarshal number 99999999999999999999 into Go struct field fixture.turns.0.fault.delay_ms of type int",
    ],
    [
      '{"prompt":"p","turns":[{"fault":5}]}',
      "json: cannot unmarshal number into Go struct field fixture.turns.0.fault of type replay.fault",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","expect":[{"is_error":"no"}]}]}',
      "json: cannot unmarshal string into Go struct field fixture.turns.0.expect.0.is_error of type bool",
    ],
    [
      '{"prompt":"p","resumes":[],"turns":[{"text":"a"}]}',
      "json: cannot unmarshal array into Go struct field fixture.resumes of type map[string]replay.segment",
    ],
    [
      '{"prompt":"p","resumes":{"1":5},"turns":[{"text":"a"}]}',
      "json: cannot unmarshal number into fixture.resumes.1 of type replay.segment",
    ],
    [
      '{"prompt":"p","resumes":{"1":{"turns":[{"text":3}]}},"turns":[{"text":"a"}]}',
      "json: cannot unmarshal number into Go struct field fixture.resumes.1.turns.0.text of type string",
    ],
    [
      '{"prompt":"p","bogus":1,"turns":[{"text":"a"}]}',
      'json: unknown field "bogus"',
    ],
    [
      '{"prompt":"p","File":"x","turns":[{"text":"a"}]}',
      'json: unknown field "File"',
    ],
    [
      '{"PROMPT":5,"turns":[{"text":"a"}]}',
      "json: cannot unmarshal number into Go struct field fixture.PROMPT of type string",
    ],
    ['{"bogus":1,"prompt":5}', 'json: unknown field "bogus"'],
    [
      '{"prompt":5,"bogus":1}',
      "json: cannot unmarshal number into Go struct field fixture.prompt of type string",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a\\x"}]}',
      "invalid escape sequence `\\x` in string",
    ],
    ['{"prompt":"p\n"}', "invalid character '\\n' in string"],
    [
      '{"prompt":"p","turns":[{"text":"a\\u12G4"}]}',
      "invalid escape sequence `\\u12G4` in string",
    ],
    [
      '{"prompt":"p","turns":[tru]}',
      "invalid character ']' in literal true (expecting 'e')",
    ],
    [
      '{"prompt":"p","turns":[01]}',
      "invalid character '1' after array element",
    ],
    ['{"prompt":"p","turns":[1.]}', "invalid character ']' in numeric literal"],
    [
      '{"prompt":"p","turns":[,]}',
      "invalid character ',' looking for beginning of value",
    ],
    [
      '{"prompt":"p","turns":[1,]}',
      "invalid character ']' looking for beginning of value",
    ],
    [
      "{'prompt':\"p\"}",
      "invalid character '\\'' looking for beginning of object key string",
    ],
    [
      '\ufeff{"prompt":"p"}',
      "invalid character '\\ufeff' looking for beginning of value",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","tool_calls":[{"name":"t","input":[1]}]}]}',
      "turns: turn 0: tool_calls[0]: input must be a JSON object",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","tool_calls":[{"name":"t","input":null}]}]}',
      "turns: turn 0: tool_calls[0]: input must be a JSON object",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","tool_calls":[{"name":"","input":{}}]}]}',
      "turns: turn 0: tool_calls[0]: name is required",
    ],
    ['{"prompt":"  ","turns":[{"text":"a"}]}', "prompt is required"],
    ['{"prompt":"p","turns":[]}', "turns needs at least one turn"],
    [
      '{"prompt":"p","turns":[{"text":"a","expect":[]}]}',
      "turns: turn 0: expect is not allowed on the first turn",
    ],
    [
      '{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"text":"b","expect":[]}]}',
      "turns: turn 1: expect has 0 entries but the previous turn has 1 tool calls",
    ],
    [
      '{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"text":"b","expect":[{}]}]}',
      "turns: turn 1: expect[0]: is_error is required",
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","fault":{}}]}',
      'turns: turn 0: fault: kind "" is not one of rate_limited, server_error, call_timeout, max_tokens, refusal, delay',
    ],
    [
      '{"prompt":"p","turns":[{"text":"a"}],"resumes":{"x\\"y":{"turns":[{"text":"a"}]}}}',
      'resumes key "x\\"y" is not a positive integer',
    ],
    [
      '{"prompt":"p","turns":[{"text":"a"}],"resumes":{"9223372036854775808":{"turns":[{"text":"a"}]}}}',
      'resumes key "9223372036854775808" is not a positive integer',
    ],
    [
      '{"prompt":"p","turns":[{"text":"a"}],"resumes":{"2":{"turns":[{"text":"a"}]},"1":{"turns":[]}}}',
      'resumes."1".turns needs at least one turn',
    ],
    [
      '{"prompt":"p","turns":[{"text":"a","fault":{"kind":"meltdown\\t\u00e9"}}]}',
      'turns: turn 0: fault: kind "meltdown\\t\u00e9" is not one of rate_limited, server_error, call_timeout, max_tokens, refusal, delay',
    ],
  ]
  for (const [content, message] of table) {
    test(JSON.stringify(content).slice(0, 80), () => {
      expect(failure(content)).toBe(`replay fixture f.json: ${message}`)
    })
  }
})

describe("promptKey", () => {
  const rows: Array<[string, string]> = [
    ["  Mixed\tCASE\n", "mixed case"],
    ["Hello World", "hello world"],
    ["Show   checkout HEALTH", "show checkout health"],
    ["İSTANBUL", "istanbul"],
    ["ΣΑΣ", "σασ"],
    ["ẞ", "ß"],
    ["a b　c d", "a b c d"],
    ["a​b", "a​b"],
    ["", ""],
  ]
  for (const [prompt, key] of rows) {
    test(JSON.stringify(prompt), () => {
      expect(promptKey(prompt)).toBe(key)
    })
  }
})

describe("goQuote matches strconv.Quote", () => {
  const rows: Array<[string, string]> = [
    ["", `""`],
    ["plain", `"plain"`],
    ['say "hi"', `"say \\"hi\\""`],
    ["back\\slash", `"back\\\\slash"`],
    ["it's <tag>&", `"it's <tag>&"`],
    ["line\nbreak\tcr\r", `"line\\nbreak\\tcr\\r"`],
    ["\u0000\u0001\u001f\u007f", `"\\x00\\x01\\x1f\\x7f"`],
    ["\u0007\b\f\u000b", `"\\a\\b\\f\\v"`],
    ["\u0080\u009f", `"\\u0080\\u009f"`],
    [" nbsp", `"\\u00a0nbsp"`],
    ["­", `"\\u00ad"`],
    ["​ zero width", `"\\u200b zero width"`],
    ["  ", `"\\u2028\\u2029"`],
    ["﻿", `"\\ufeff"`],
    ["\u{e0001}", `"\\U000e0001"`],
    ["\u{10ffff}", `"\\U0010ffff"`],
    ["͸", `"\\u0378"`],
    ["é ü 日本語 \u{1f600}", `"é ü 日本語 \u{1f600}"`],
  ]
  for (const [text, quoted] of rows) {
    test(quoted, () => {
      expect(goQuote(text)).toBe(quoted)
    })
  }
})

describe("raw JSON scanner", () => {
  test("keeps braces inside strings out of the structure", () => {
    const { node } = parseJsonDocument(`{"a":"}{","b":[1]}`)
    expect(node.kind).toBe("object")
    expect(node.raw).toBe(`{"a":"}{","b":[1]}`)
  })

  test("handles escaped quotes and backslashes", () => {
    const { node } = parseJsonDocument(`{"a":"x\\"}\\\\","b":2}`)
    if (node.kind !== "object") {
      throw new Error("not an object")
    }
    expect(node.members.map((m) => m.key)).toEqual(["a", "b"])
    expect(node.members[0]?.value.raw).toBe(`"x\\"}\\\\"`)
  })

  test("keeps multi-byte runes and unicode escapes intact", () => {
    const { node } = parseJsonDocument(`{"ké":"日本 \\u00e9 \\ud83d\\ude00"}`)
    if (node.kind !== "object") {
      throw new Error("not an object")
    }
    expect(node.members[0]?.key).toBe("ké")
    const value = node.members[0]?.value
    expect(value?.kind === "string" ? value.value : "").toBe("日本 é \u{1f600}")
  })

  test("reports content after the document", () => {
    expect(parseJsonDocument(`{} x`).trailing).toBe(true)
    expect(parseJsonDocument(`{}  \n`).trailing).toBe(false)
  })
})
