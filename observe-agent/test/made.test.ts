import { describe, expect, test } from "bun:test"
import { madeAnswer, madeBy, maxMadeLines } from "../src/made"
import type { ToolSpec } from "../src/session/types"

const specs: ToolSpec[] = [
  {
    name: "create_observation",
    description: "",
    inputSchema: {},
    readOnly: false,
  },
  { name: "create_folder", description: "", inputSchema: {}, readOnly: false },
  { name: "list_dashboards", description: "", inputSchema: {}, readOnly: true },
  { name: "save_finding", description: "", inputSchema: {}, readOnly: false },
  { name: "pin_dashboard", description: "", inputSchema: {} },
]

function call(name: string, input: object = {}) {
  return { id: name, name, input: JSON.stringify(input) }
}

function result(content: object | string, isError = false) {
  return {
    callId: "c",
    content: typeof content === "string" ? content : JSON.stringify(content),
    isError,
    prunable: false,
  }
}

describe("madeBy", () => {
  test("keeps successful writes with the input title and the output url", () => {
    const made = madeBy(
      specs,
      [
        call("create_observation", { title: "Checkout latency" }),
        call("create_folder", { name: "Team" }),
      ],
      [
        result(
          `${JSON.stringify({ url: "http://app/o/1" })}\n\n<note>kept</note>`
        ),
        result({ id: "f1" }),
      ]
    )
    expect(made).toEqual([
      {
        tool: "create_observation",
        title: "Checkout latency",
        url: "http://app/o/1",
      },
      { tool: "create_folder", title: "Team", url: "" },
    ])
  })

  test("skips reads, saves, failed writes and tools not known to write", () => {
    const made = madeBy(
      specs,
      [
        call("list_dashboards"),
        call("save_finding", { markdown: "x" }),
        call("create_observation", { title: "t" }),
        call("pin_dashboard"),
      ],
      [
        result({}),
        result({}),
        result({ code: "out_of_scope" }, true),
        result({}),
      ]
    )
    expect(made).toEqual([])
  })

  test("reads a result that is not JSON as having no url", () => {
    const made = madeBy(specs, [call("create_observation")], [result("ok")])
    expect(made).toEqual([{ tool: "create_observation", title: "", url: "" }])
  })
})

describe("madeAnswer", () => {
  test("links each made thing by its title", () => {
    expect(
      madeAnswer([
        {
          tool: "create_observation",
          title: "Latency [p95]",
          url: "http://app/o/1",
        },
        { tool: "create_folder", title: "Team", url: "" },
        { tool: "add_block", title: "", url: "http://app/o/1" },
        { tool: "pin_observation", title: "", url: "" },
      ])
    ).toBe(
      "Done. This run made what you asked for:\n\n" +
        "- Create observation: [Latency \\[p95\\]](http://app/o/1)\n" +
        "- Create folder: Team\n" +
        "- Add block: http://app/o/1\n" +
        "- Pin observation"
    )
  })

  test("escapes backslashes and brackets in a title", () => {
    expect(
      madeAnswer([
        { tool: "create_folder", title: "a\\[b]\\", url: "http://app/f/1" },
      ])
    ).toContain("- Create folder: [a\\\\\\[b\\]\\\\](http://app/f/1)")
  })

  test("counts what is past the line cap", () => {
    const made = Array.from({ length: maxMadeLines + 2 }, () => ({
      tool: "add_block",
      title: "",
      url: "",
    }))
    const lines = madeAnswer(made).split("\n")
    expect(lines.at(-1)).toBe("- and 2 more")
    expect(lines.filter((l) => l === "- Add block")).toHaveLength(maxMadeLines)
  })
})
