import { describe, expect, test } from "bun:test"
import { createLogger, parseLevel } from "../src/log"

function capture(threshold: Parameters<typeof createLogger>[1]) {
  const lines: string[] = []
  const logger = createLogger(
    (line) => lines.push(line),
    threshold,
    () => new Date("2026-11-12T10:00:00.000Z")
  )
  return { lines, logger }
}

describe("log", () => {
  test("writes one JSON object per line with time, level and msg", () => {
    const { lines, logger } = capture("debug")
    logger("info", "run claimed", { slot: 2, replay: true, kind: "observe" })
    expect(lines).toHaveLength(1)
    expect(lines[0]).not.toContain("\n")
    expect(JSON.parse(lines[0] ?? "")).toEqual({
      time: "2026-11-12T10:00:00.000Z",
      level: "info",
      msg: "run claimed",
      slot: 2,
      replay: true,
      kind: "observe",
    })
  })

  test("drops lines below the configured level", () => {
    const cases = [
      { threshold: "debug", emitted: ["debug", "info", "warn", "error"] },
      { threshold: "info", emitted: ["info", "warn", "error"] },
      { threshold: "warn", emitted: ["warn", "error"] },
      { threshold: "error", emitted: ["error"] },
    ] as const
    for (const c of cases) {
      const { lines, logger } = capture(c.threshold)
      for (const level of ["debug", "info", "warn", "error"] as const) {
        logger(level, level)
      }
      expect(lines.map((l) => JSON.parse(l).level)).toEqual([...c.emitted])
    }
  })

  test("fields cannot overwrite the keys the server reads", () => {
    const { lines, logger } = capture("info")
    logger("info", "real", { level: "error", msg: "fake", component: "x" })
    expect(JSON.parse(lines[0] ?? "")).toMatchObject({
      level: "info",
      msg: "real",
    })
    expect(JSON.parse(lines[0] ?? "")).not.toHaveProperty("component")
  })

  test("parses level names", () => {
    expect(parseLevel("WARN")).toBe("warn")
    expect(parseLevel(" debug ")).toBe("debug")
    expect(parseLevel("verbose")).toBeUndefined()
    expect(parseLevel(undefined)).toBeUndefined()
  })
})
