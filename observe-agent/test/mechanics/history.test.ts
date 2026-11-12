import { describe, expect, test } from "bun:test"
import type { ModelMessage } from "ai"
import { Pruner, prunedResult } from "../../src/session/mechanics/compact"
import {
  assistantMessage,
  checkResults,
  cutToolResults,
  endsInPlainAnswer,
  messageBytes,
  pruneView,
  toolMessage,
  userMessage,
  weigh,
  withStubs,
  withoutReasoning,
} from "../../src/session/mechanics/history"
import {
  SessionError,
  type ToolCall,
  type ToolResult,
} from "../../src/session/types"

const calls: ToolCall[] = [
  { id: "c1", name: "list", input: "{}" },
  { id: "c2", name: "save", input: "{}" },
]

function result(callId: string, content: string, isError = false): ToolResult {
  return { callId, content, isError, prunable: !isError }
}

describe("checkResults", () => {
  test("accepts one result per pending call, in order", () => {
    expect(() =>
      checkResults(
        "anthropic",
        false,
        ["c1", "c2"],
        [result("c1", "a"), result("c2", "b")]
      )
    ).not.toThrow()
    expect(() => checkResults("openai", false, [], [])).not.toThrow()
  })

  const rejected: Array<[string, boolean, string[], ToolResult[], string]> = [
    [
      "an ended session",
      true,
      [],
      [],
      "anthropic session: the previous turn made no tool calls, so the session has ended",
    ],
    [
      "too few results",
      false,
      ["c1", "c2"],
      [result("c1", "a")],
      "anthropic session: got 1 tool results for 2 tool calls",
    ],
    [
      "results with none pending",
      false,
      [],
      [result("c1", "a")],
      "anthropic session: got 1 tool results for 0 tool calls",
    ],
    [
      "results out of order",
      false,
      ["c1", "c2"],
      [result("c2", "b"), result("c1", "a")],
      'anthropic session: tool result 0 answers "c2", want "c1"',
    ],
  ]
  test.each(rejected)(
    "rejects %s",
    (_name, ended, pending, results, message) => {
      expect(() => checkResults("anthropic", ended, pending, results)).toThrow(
        SessionError
      )
      expect(() => checkResults("anthropic", ended, pending, results)).toThrow(
        message
      )
    }
  )
})

describe("toolMessage", () => {
  const results = [result("c1", "ok"), result("c2", "boom", true)]

  test("the flag style marks an error result as an error", () => {
    expect(toolMessage("flag", calls, results)).toEqual({
      role: "tool",
      content: [
        {
          type: "tool-result",
          toolCallId: "c1",
          toolName: "list",
          output: { type: "text", value: "ok" },
        },
        {
          type: "tool-result",
          toolCallId: "c2",
          toolName: "save",
          output: { type: "error-text", value: "boom" },
        },
      ],
    })
  })

  test("the label style prefixes an error result with ERROR: ", () => {
    const message = toolMessage("label", calls, results)
    expect(
      message.content.map((part) => part.type === "tool-result" && part.output)
    ).toEqual([
      { type: "text", value: "ok" },
      { type: "text", value: "ERROR: boom" },
    ])
  })
})

describe("messages", () => {
  test("the first user message is the text as given", () => {
    expect(userMessage("hello")).toEqual({
      role: "user",
      content: [{ type: "text", text: "hello" }],
    })
  })

  const plain = assistantMessage([{ type: "text", text: "done" }])
  const withCall = assistantMessage([
    {
      type: "reasoning",
      text: "hmm",
      providerOptions: { anthropic: { signature: "s" } },
    },
    { type: "tool-call", toolCallId: "c1", toolName: "list", input: {} },
  ])

  test("endsInPlainAnswer is true only for an assistant message with no tool call", () => {
    expect(endsInPlainAnswer([userMessage("x"), plain])).toBe(true)
    expect(endsInPlainAnswer([userMessage("x"), withCall])).toBe(false)
    expect(endsInPlainAnswer([plain, userMessage("x")])).toBe(false)
    expect(endsInPlainAnswer([])).toBe(false)
  })

  test("withoutReasoning drops reasoning parts only", () => {
    expect(withoutReasoning(withCall)).toEqual(
      assistantMessage([
        { type: "tool-call", toolCallId: "c1", toolName: "list", input: {} },
      ])
    )
    expect(withoutReasoning(userMessage("x"))).toEqual(userMessage("x"))
  })
})

describe("bytes", () => {
  test("results count in UTF-8 bytes, capped at the result limit", () => {
    const message = toolMessage("flag", calls, [
      result("c1", "é".repeat(10)),
      result("c2", "abc"),
    ])
    expect(messageBytes(message, 0)).toBe(23)
    expect(messageBytes(message, 5)).toBe(8)
  })

  test("an assistant message counts text and tool-call input, not reasoning", () => {
    const message = assistantMessage([
      { type: "reasoning", text: "x".repeat(100) },
      { type: "text", text: "héllo" },
      {
        type: "tool-call",
        toolCallId: "c1",
        toolName: "save",
        input: { a: 1 },
      },
    ])
    expect(messageBytes(message, 0)).toBe(6 + 7)
  })

  test("weigh marks the assistant messages", () => {
    const weighed = weigh(
      [userMessage("ab"), assistantMessage([{ type: "text", text: "abc" }])],
      0
    )
    expect(weighed).toEqual([
      { assistant: false, bytes: 2 },
      { assistant: true, bytes: 3 },
    ])
  })
})

describe("pruning and cutting results", () => {
  const history: ModelMessage[] = [
    userMessage("first"),
    assistantMessage([
      { type: "tool-call", toolCallId: "c1", toolName: "list", input: {} },
    ]),
    toolMessage("flag", [calls[0]!], [result("c1", "x".repeat(50))]),
    assistantMessage([
      { type: "tool-call", toolCallId: "c2", toolName: "save", input: {} },
    ]),
    toolMessage("flag", [calls[1]!], [result("c2", "saved")]),
  ]

  test("pruneView lists each result with its bytes", () => {
    expect(pruneView(history).map((m) => m.results)).toEqual([
      [],
      [],
      [{ id: "c1", bytes: 50 }],
      [],
      [{ id: "c2", bytes: 5 }],
    ])
  })

  test("withStubs replaces only prunable results before the boundary", () => {
    const pruner = new Pruner({ keepTurns: 1, stepBytes: 1 })
    pruner.note([result("c1", "x"), { ...result("c2", "x"), prunable: false }])
    const stubbed = withStubs(history, 3, pruner, "flag")
    expect(stubbed[2]).toEqual(
      toolMessage("flag", [calls[0]!], [result("c1", prunedResult)])
    )
    expect(stubbed[4]).toBe(history[4]!)
    expect(withStubs(history, 0, pruner, "flag")).toBe(history)
  })

  test("a stub keeps the error mark of its style", () => {
    const pruner = new Pruner({ keepTurns: 1, stepBytes: 1 })
    pruner.note([result("c1", "x")])
    const flagged = [
      toolMessage("flag", [calls[0]!], [result("c1", "bad", true)]),
    ]
    const labelled = [
      toolMessage("label", [calls[0]!], [result("c1", "bad", true)]),
    ]
    expect(withStubs(flagged, 1, pruner, "flag")[0]).toEqual(
      toolMessage("flag", [calls[0]!], [result("c1", prunedResult, true)])
    )
    expect(withStubs(labelled, 1, pruner, "label")[0]).toEqual(
      toolMessage("label", [calls[0]!], [result("c1", prunedResult, true)])
    )
  })

  test("cutToolResults cuts every result to the byte limit and leaves other messages", () => {
    const cut = cutToolResults(history[2]!, 10)
    expect(messageBytes(cut, 0)).toBeGreaterThan(10)
    const part = (cut as { content: Array<{ output: { value: string } }> })
      .content[0]!
    expect(part.output.value).toBe(
      "x".repeat(10) +
        "\n[cut from 50 bytes; call the tool again for the full result]"
    )
    expect(cutToolResults(history[1]!, 10)).toBe(history[1]!)
  })
})
