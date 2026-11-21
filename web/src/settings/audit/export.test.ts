import { describe, expect, test } from "vitest"

import {
  RowCounter,
  exportFileName,
  exportOutcome,
  readExport,
  rowLimitOf,
} from "./export"

const encoder = new TextEncoder()

function bytes(text: string): Uint8Array<ArrayBuffer> {
  return encoder.encode(text)
}

function body(
  parts: string[],
  failAfter = false
): ReadableStream<Uint8Array<ArrayBuffer>> {
  let index = 0
  return new ReadableStream({
    pull(controller) {
      if (index < parts.length) {
        controller.enqueue(bytes(parts[index++]))
        return
      }
      if (failAfter) controller.error(new TypeError("network error"))
      else controller.close()
    },
  })
}

describe("RowCounter", () => {
  test("counts NDJSON lines across chunks", () => {
    const counter = new RowCounter("ndjson")
    counter.push(bytes('{"id":"a"}\n{"id"'))
    counter.push(bytes(':"b"}\n'))
    expect(counter.rows()).toBe(2)
  })

  test("leaves out the CSV header and newlines inside quoted cells", () => {
    const counter = new RowCounter("csv")
    counter.push(bytes('id,data\na,"{""note"":""line one\n'))
    counter.push(bytes('line two""}"\nb,{}\n'))
    expect(counter.rows()).toBe(2)
  })

  test("keeps a multi-byte letter split across chunks whole", () => {
    const counter = new RowCounter("csv")
    const all = bytes('id,name\na,"Zoë\n"\n')
    const split = all.indexOf(0xc3) + 1
    counter.push(all.slice(0, split))
    counter.push(all.slice(split))
    expect(counter.rows()).toBe(1)
  })
})

describe("readExport", () => {
  test("reads a whole body", async () => {
    const read = await readExport(body(["{}\n", "{}\n"]), "ndjson")
    expect(read).toMatchObject({ rows: 2, cutOff: false })
    expect(read.chunks).toHaveLength(2)
  })

  test("keeps the rows that arrived before the connection was cut", async () => {
    const read = await readExport(body(["{}\n{}\n", "{}\n"], true), "ndjson")
    expect(read).toMatchObject({ rows: 3, cutOff: true })
  })
})

describe("exportFileName", () => {
  const now = new Date("2026-11-21T06:30:00.123Z")

  test("uses the server's file name", () => {
    expect(
      exportFileName(
        'attachment; filename="audit-20261121T063000Z.csv"',
        "csv",
        now,
        false
      )
    ).toBe("audit-20261121T063000Z.csv")
  })

  test("marks an incomplete file before its extension", () => {
    expect(
      exportFileName(
        'attachment; filename="audit-x.ndjson"',
        "ndjson",
        now,
        true
      )
    ).toBe("audit-x-incomplete.ndjson")
  })

  test("falls back to a name of its own", () => {
    expect(exportFileName(null, "csv", now, false)).toBe(
      "audit-2026-11-21T06-30-00-123Z.csv"
    )
  })
})

describe("exportOutcome", () => {
  test("is complete below the limit", () => {
    expect(
      exportOutcome({ rows: 12, cutOff: false }, 1_000_000, "a.csv")
    ).toEqual({ kind: "complete", rows: 12 })
  })

  test("says more events matched at the limit", () => {
    const outcome = exportOutcome(
      { rows: 1_000_000, cutOff: false },
      1_000_000,
      "a.csv"
    )
    expect(outcome.kind).toBe("limit")
    expect(outcome).toMatchObject({
      message: expect.stringContaining(
        "1,000,000 events, and more events matched"
      ),
    })
  })

  test("says a cut-off file is incomplete", () => {
    const outcome = exportOutcome(
      { rows: 1, cutOff: true },
      1_000_000,
      "a-incomplete.csv"
    )
    expect(outcome.kind).toBe("incomplete")
    expect(outcome).toMatchObject({
      message: expect.stringContaining(
        "a-incomplete.csv is incomplete: it holds only the first 1 event"
      ),
    })
  })
})

describe("rowLimitOf", () => {
  test("reads a positive integer", () => {
    expect(rowLimitOf("1000000")).toBe(1_000_000)
    expect(rowLimitOf(null)).toBe(null)
    expect(rowLimitOf("many")).toBe(null)
  })
})
