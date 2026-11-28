import { describe, expect, test } from "bun:test"
import { existsSync, readFileSync } from "node:fs"
import { join, resolve } from "node:path"
import { offersWrite, saveTools } from "../src/phase"

const toolsDir = resolve(
  import.meta.dir,
  "..",
  "..",
  "internal",
  "agent",
  "tools"
)

describe("save tools", () => {
  test("are the tools the server counts as saves", () => {
    expect([...saveTools].sort()).toEqual([
      "create_dashboard",
      "import_grafana_dashboard",
      "save_finding",
      "update_dashboard",
      "update_panels",
    ])
  })

  const phase = join(toolsDir, "phase.go")
  test.skipIf(!existsSync(phase))("match tools.Phase in the Go source", () => {
    const source = readFileSync(phase, "utf8")
    const saveCase = source.match(
      /var saveTools = map\[string\]bool\{([^}]+)\}/
    )
    expect(saveCase).not.toBeNull()
    const constants = [...(saveCase?.[1] ?? "").matchAll(/(name\w+):/g)].map(
      (match) => match[1] as string
    )
    const declared = new Map<string, string>()
    for (const file of ["schemas.go", "finding.go"]) {
      const text = readFileSync(join(toolsDir, file), "utf8")
      for (const match of text.matchAll(/(name\w+)\s*=\s*"([^"]+)"/g)) {
        declared.set(match[1] as string, match[2] as string)
      }
    }
    const names = constants.map((c) => declared.get(c))
    expect(names.every((name) => name !== undefined)).toBe(true)
    expect([...names].sort()).toEqual([...saveTools].sort())
  })
})

describe("offersWrite", () => {
  const spec = (name: string, readOnly?: boolean) => ({
    name,
    description: "",
    inputSchema: {},
    ...(readOnly === undefined ? {} : { readOnly }),
  })

  test("is false for reads, saves and tools that do not say", () => {
    expect(
      offersWrite([
        spec("list_datasources", true),
        spec("create_dashboard", false),
        spec("save_finding", false),
        spec("unannotated"),
      ])
    ).toBe(false)
  })

  test("is true for a non-save tool marked as not read-only", () => {
    expect(offersWrite([spec("pin_observation", false)])).toBe(true)
  })
})
