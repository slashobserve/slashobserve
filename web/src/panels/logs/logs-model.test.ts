import { describe, expect, it } from "vitest"

import { formatDateTime } from "@/lib/time"
import { createFieldResolver } from "@/panels/field-config"
import type { PreparedField, PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  detailText,
  logLevel,
  logsModel,
  maxEntries,
  type LogsDisplay,
} from "./logs-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "logs",
    datasource: "ch",
    queries: [{ query: "SELECT 1", format: "logs" }],
    ...extra,
  } as DataPanel
}

function field(
  name: string,
  type: PreparedField["type"],
  values: unknown[]
): PreparedField {
  return { name, type, values }
}

function frame(
  fields: PreparedField[],
  extra: Partial<PreparedFrame> = {}
): PreparedFrame {
  return { ref: "A", fields, ...extra }
}

function model(
  frames: PreparedFrame[],
  display: Partial<LogsDisplay> = {},
  extra: Partial<DataPanel> = {}
) {
  const p = panel(extra)
  return logsModel(
    frames,
    createFieldResolver(p),
    displayWithDefaults("logs", display) as LogsDisplay
  )
}

const stamps = [3000, 1000, 2000]
const bodies = ["third", "first", "second"]

describe("logsModel columns", () => {
  it.each(["Body", "message", "MSG", "line", "Log"])(
    "takes the message from a field named %s",
    (name) => {
      const { entries } = model([
        frame([
          field("Timestamp", "time", [1000]),
          field("ServiceName", "string", ["checkout"]),
          field(name, "string", ["hello"]),
        ]),
      ])
      expect(entries[0].message).toBe("hello")
    }
  )

  it.each(["SeverityText", "severity", "Level", "LVL", "detected_level"])(
    "takes the level from a field named %s",
    (name) => {
      const { entries } = model([
        frame([
          field("Body", "string", ["hello"]),
          field(name, "string", ["warn"]),
        ]),
      ])
      expect(entries[0].level).toEqual({ word: "Warning", kind: "warning" })
      expect(entries[0].message).toBe("hello")
    }
  )

  it("takes the time from the first time field", () => {
    const { entries } = model([
      frame([
        field("Body", "string", ["hello"]),
        field("first", "time", [1500]),
        field("second", "time", [9000]),
      ]),
    ])
    expect(entries[0].time).toBe(1500)
  })

  it("falls back to the first string field that is not the level", () => {
    const { entries } = model([
      frame([
        field("Timestamp", "time", [1000]),
        field("severity", "string", ["info"]),
        field("service", "string", ["checkout"]),
        field("other", "string", ["x"]),
      ]),
    ])
    expect(entries[0].message).toBe("checkout")
    expect(entries[0].level?.kind).toBe("info")
  })

  it("prefers a named message over an earlier string field", () => {
    const { entries } = model([
      frame([
        field("service", "string", ["checkout"]),
        field("body", "string", ["hello"]),
      ]),
    ])
    expect(entries[0].message).toBe("hello")
  })

  it("uses a value's display text when a frame has no string field", () => {
    const { entries } = model(
      [
        frame(
          [
            field("Time", "time", [1000, 2000]),
            field("Value", "number", [1536, null]),
          ],
          { name: "up", labels: { job: "api" } }
        ),
      ],
      {},
      { unit: "bytes", noValue: "none" }
    )
    expect(entries.map((entry) => entry.message)).toEqual([
      'up{job="api"} none',
      'up{job="api"} 1.5 KiB',
    ])
    expect(entries.every((entry) => entry.level === null)).toBe(true)
  })

  it("keeps the value alone when a frame has no series name or labels", () => {
    const { entries } = model([
      frame([field("Time", "time", [1000]), field("Value", "number", [3])]),
    ])
    expect(entries[0].message).toBe("3")
  })

  it("tells the series of two Prometheus frames apart", () => {
    const series = (job: string, value: number) =>
      frame(
        [field("Time", "time", [1000]), field("Value", "number", [value])],
        { ref: "A", name: "up", labels: { job } }
      )
    const { entries } = model([series("j1", 1), series("j2", 2)])
    const byMessage = new Map(
      entries.map((entry) => [entry.message, entry] as const)
    )
    expect([...byMessage.keys()].sort()).toEqual([
      'up{job="j1"} 1',
      'up{job="j2"} 2',
    ])
    const labelsOf = (message: string) =>
      byMessage
        .get(message)
        ?.details.filter((detail) => "text" in detail)
        .map((detail) => [detail.name, "text" in detail ? detail.text : ""])
    expect(labelsOf('up{job="j1"} 1')).toEqual([
      ["__name__", "up"],
      ["job", "j1"],
    ])
    expect(labelsOf('up{job="j2"} 2')).toEqual([
      ["__name__", "up"],
      ["job", "j2"],
    ])
  })

  it("shows the series name and labels as detail rows", () => {
    const fields = createFieldResolver(panel())
    const { entries } = model([
      frame([field("Time", "time", [1000]), field("Value", "number", [1])], {
        name: "up",
        labels: { pod: "a", job: "j1" },
      }),
    ])
    expect(
      entries[0].details.map((detail) => [
        detail.name,
        detailText(fields, detail),
      ])
    ).toEqual([
      ["Time", formatDateTime(1000)],
      ["Value", "1"],
      ["__name__", "up"],
      ["job", "j1"],
      ["pod", "a"],
    ])
  })

  it("leaves the message empty when the only string field is the level", () => {
    const { entries } = model([frame([field("level", "string", ["error"])])])
    expect(entries[0]).toMatchObject({ message: "", level: { kind: "error" } })
  })

  it("gives a null message value an empty message", () => {
    const { entries } = model([frame([field("Body", "string", [null, "x"])])])
    expect(entries.map((entry) => entry.message).sort()).toEqual(["", "x"])
  })

  it("lists every field as a detail, message and level included", () => {
    const f = frame([
      field("Timestamp", "time", [1000]),
      field("SeverityText", "string", ["Error"]),
      field("ServiceName", "string", ["checkout"]),
      field("Body", "string", ["boom"]),
    ])
    const { entries } = model([f])
    expect(entries[0].details.map((detail) => detail.name)).toEqual([
      "Timestamp",
      "SeverityText",
      "ServiceName",
      "Body",
    ])
    expect(entries[0].details[2]).toMatchObject({
      frame: f,
      field: f.fields[2],
      index: 0,
    })
  })

  it("makes one entry per row of every frame", () => {
    const { entries } = model([
      frame([field("Body", "string", ["a", "b"])], { ref: "A" }),
      frame([field("Body", "string", ["c"])], { ref: "B" }),
    ])
    expect(entries.map((entry) => entry.message).sort()).toEqual([
      "a",
      "b",
      "c",
    ])
  })
})

describe("logsModel order", () => {
  const f = () =>
    frame([field("Timestamp", "time", stamps), field("Body", "string", bodies)])

  it("puts the newest line first by default", () => {
    expect(model([f()]).entries.map((entry) => entry.message)).toEqual([
      "third",
      "second",
      "first",
    ])
  })

  it("puts the oldest line first when asked", () => {
    expect(
      model([f()], { order: "oldest" }).entries.map((entry) => entry.message)
    ).toEqual(["first", "second", "third"])
  })

  it.each(["newest", "oldest"] as const)(
    "keeps lines without a time at the end in their order when %s first",
    (order) => {
      const { entries } = model(
        [
          frame([
            field("Timestamp", "time", [null, 2000, null, 1000]),
            field("Body", "string", ["n1", "t2", "n2", "t1"]),
          ]),
        ],
        { order }
      )
      const messages = entries.map((entry) => entry.message)
      expect(messages.slice(2)).toEqual(["n1", "n2"])
      expect(messages.slice(0, 2)).toEqual(
        order === "newest" ? ["t2", "t1"] : ["t1", "t2"]
      )
    }
  )

  it("orders lines across frames", () => {
    const { entries } = model([
      frame([
        field("Timestamp", "time", [1000, 3000]),
        field("Body", "string", ["a1", "a3"]),
      ]),
      frame(
        [field("Timestamp", "time", [2000]), field("Body", "string", ["b2"])],
        { ref: "B" }
      ),
    ])
    expect(entries.map((entry) => entry.message)).toEqual(["a3", "b2", "a1"])
  })

  it("treats a time outside the date range as missing, not as an error", () => {
    const { entries } = model([
      frame([
        field("Timestamp", "time", [1e300, Infinity, NaN, 1000]),
        field("Body", "string", ["huge", "inf", "nan", "ok"]),
      ]),
    ])
    expect(entries.map((entry) => entry.time)).toEqual([1000, null, null, null])
    for (const entry of entries) {
      if (entry.time !== null) {
        expect(() => new Date(entry.time as number).toISOString()).not.toThrow()
      }
    }
  })
})

describe("logsModel cap", () => {
  function many(count: number) {
    return frame([
      field(
        "Timestamp",
        "time",
        Array.from({ length: count }, (_, i) => i)
      ),
      field(
        "Body",
        "string",
        Array.from({ length: count }, (_, i) => `line ${i}`)
      ),
    ])
  }

  it("keeps the newest lines and flags the cut", () => {
    const { entries, truncated } = model([many(maxEntries + 5)])
    expect(truncated).toBe(true)
    expect(entries).toHaveLength(maxEntries)
    expect(entries[0].message).toBe(`line ${maxEntries + 4}`)
    expect(entries.at(-1)?.message).toBe("line 5")
  })

  it("keeps the oldest lines when oldest comes first", () => {
    const { entries, truncated } = model([many(maxEntries + 5)], {
      order: "oldest",
    })
    expect(truncated).toBe(true)
    expect(entries[0].message).toBe("line 0")
    expect(entries.at(-1)?.message).toBe(`line ${maxEntries - 1}`)
  })

  it("does not flag exactly the cap", () => {
    const { entries, truncated } = model([many(maxEntries)])
    expect(truncated).toBe(false)
    expect(entries).toHaveLength(maxEntries)
  })

  it("handles a row count far past the cap", () => {
    const { entries, truncated } = model([many(200_000)])
    expect(truncated).toBe(true)
    expect(entries).toHaveLength(maxEntries)
  })
})

describe("logsModel keys", () => {
  it("gives identical lines distinct keys", () => {
    const { entries } = model([
      frame([
        field("Timestamp", "time", [1000, 1000]),
        field("Body", "string", ["same", "same"]),
      ]),
    ])
    expect(new Set(entries.map((entry) => entry.key)).size).toBe(2)
  })

  it("keeps a line's key when newer lines arrive above it", () => {
    const old = model([
      frame([
        field("Timestamp", "time", [1000]),
        field("Body", "string", ["x"]),
      ]),
    ]).entries[0].key
    const next = model([
      frame([
        field("Timestamp", "time", [2000, 1000]),
        field("Body", "string", ["y", "x"]),
      ]),
    ]).entries
    expect(next[1].key).toBe(old)
  })
})

describe("logLevel", () => {
  it.each([
    ["emerg", "Critical", "error"],
    ["fatal", "Critical", "error"],
    ["alert", "Critical", "error"],
    ["crit", "Critical", "error"],
    ["Critical", "Critical", "error"],
    ["PANIC", "Critical", "error"],
    ["error", "Error", "error"],
    ["ERR", "Error", "error"],
    ["eror", "Error", "error"],
    ["warn", "Warning", "warning"],
    ["WARNING", "Warning", "warning"],
    ["info", "Info", "info"],
    ["information", "Info", "info"],
    ["informational", "Info", "info"],
    ["notice", "Info", "info"],
    ["dbug", "Debug", "other"],
    ["DEBUG", "Debug", "other"],
    ["trace", "Trace", "other"],
    ["verbose", "Verbose", "other"],
    ["VERBOSE", "Verbose", "other"],
  ])("reads %s as the word %s of kind %s", (text, word, kind) => {
    expect(logLevel(text)).toEqual({ word, kind })
  })

  it("shows WARN and warning as the same word", () => {
    expect(logLevel("WARN")?.word).toBe(logLevel("warning")?.word)
  })

  it("trims the text and keeps a number's digits", () => {
    expect(logLevel("  error ")).toEqual({ word: "Error", kind: "error" })
    expect(logLevel(17)).toEqual({ word: "17", kind: "other" })
  })

  it.each([null, undefined, "", "   "])("has no level for %j", (value) => {
    expect(logLevel(value)).toBeNull()
  })

  it.each(["constructor", "__proto__", "toString", "hasOwnProperty"])(
    "reads the inherited object key %s as an unknown level",
    (text) => {
      expect(logLevel(text)?.kind).toBe("other")
    }
  )
})

describe("logsModel severity numbers", () => {
  const levelsFor = (numbers: unknown[], extra: PreparedField[] = []) =>
    model([
      frame([
        field(
          "Body",
          "string",
          numbers.map(() => "x")
        ),
        field("SeverityNumber", "number", numbers),
        ...extra,
      ]),
    ]).entries.map((entry) => entry.level?.word ?? null)

  it("maps the OpenTelemetry bands when there is no level text", () => {
    const numbers = [1, 4, 5, 8, 9, 12, 13, 16, 17, 20, 21, 24]
    expect(levelsFor(numbers)).toEqual([
      "Trace",
      "Trace",
      "Debug",
      "Debug",
      "Info",
      "Info",
      "Warning",
      "Warning",
      "Error",
      "Error",
      "Critical",
      "Critical",
    ])
  })

  it("gives numbers outside 1 to 24 no level", () => {
    expect(levelsFor([0, 25, -3, 9.5, null])).toEqual([
      null,
      null,
      null,
      null,
      null,
    ])
  })

  it("prefers the level text over the number", () => {
    const { entries } = model([
      frame([
        field("Body", "string", ["a", "b"]),
        field("SeverityText", "string", ["ERROR", ""]),
        field("SeverityNumber", "number", [9, 9]),
      ]),
    ])
    expect(entries.map((entry) => entry.level?.word).sort()).toEqual([
      "Error",
      "Info",
    ])
  })
})

describe("detailText", () => {
  function detailOf(
    f: PreparedFrame,
    name: string,
    extra: Partial<DataPanel> = {}
  ) {
    const fields = createFieldResolver(panel(extra))
    const target = f.fields.find((candidate) => candidate.name === name)
    if (!target) throw new Error(`no field ${name}`)
    return detailText(fields, { name, field: target, frame: f, index: 0 })
  }

  const f = frame([
    field("Timestamp", "time", [1700000000000]),
    field("Status", "number", [503]),
    field("Size", "number", [1536]),
    field("Body", "string", ["<b>boom</b>"]),
  ])

  it("writes a time field as a date and time", () => {
    expect(detailOf(f, "Timestamp")).toBe(formatDateTime(1700000000000))
  })

  it("applies the field's own config and mappings", () => {
    expect(
      detailOf(f, "Size", {
        overrides: [{ match: { field: "Size" }, unit: "bytes" }],
      } as Partial<DataPanel>)
    ).toBe("1.5 KiB")
    expect(
      detailOf(f, "Status", {
        overrides: [
          {
            match: { field: "Status" },
            mappings: [{ value: "503", text: "Unavailable" }],
          },
        ],
      } as Partial<DataPanel>)
    ).toBe("Unavailable")
  })

  it("keeps a mapped time field's mapping instead of a date", () => {
    expect(
      detailOf(f, "Timestamp", {
        overrides: [
          {
            match: { field: "Timestamp" },
            mappings: [{ value: "1700000000000", text: "then" }],
          },
        ],
      } as Partial<DataPanel>)
    ).toBe("then")
  })

  it("returns markup in a message as plain text", () => {
    expect(detailOf(f, "Body")).toBe("<b>boom</b>")
    expect(model([f]).entries[0].message).toBe("<b>boom</b>")
  })
})
