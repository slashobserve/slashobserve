import { describe, expect, test } from "bun:test"
import { mcpUrl, parseEnv } from "../src/env"

const valid = {
  HARNESS_SERVER_URL: "http://127.0.0.1:8080",
  HARNESS_TOKEN: "sob_secret",
  HARNESS_SLOTS: "4",
  HARNESS_LOG_LEVEL: "debug",
}

describe("env", () => {
  test("reads the four variables", () => {
    const result = parseEnv(valid)
    expect(result.ok).toBe(true)
    if (!result.ok) {
      return
    }
    expect(result.env.serverUrl.href).toBe("http://127.0.0.1:8080/")
    expect(result.env.token).toBe("sob_secret")
    expect(result.env.slots).toBe(4)
    expect(result.env.logLevel).toBe("debug")
  })

  test("slots default to one and the level to info", () => {
    const result = parseEnv({
      HARNESS_SERVER_URL: valid.HARNESS_SERVER_URL,
      HARNESS_TOKEN: valid.HARNESS_TOKEN,
    })
    expect(result.ok && result.env.slots).toBe(1)
    expect(result.ok && result.env.logLevel).toBe("info")
  })

  test("refuses what the server would never send", () => {
    const cases: Array<[string, Record<string, string>, string]> = [
      ["missing url", { HARNESS_SERVER_URL: "" }, "SERVER_URL is not set"],
      [
        "url without a scheme",
        { HARNESS_SERVER_URL: "127.0.0.1:8080" },
        "http or https",
      ],
      [
        "non http scheme",
        { HARNESS_SERVER_URL: "ftp://example.com" },
        "http or https",
      ],
      ["missing token", { HARNESS_TOKEN: " " }, "TOKEN is not set"],
      ["zero slots", { HARNESS_SLOTS: "0" }, "from 1 to 64"],
      ["too many slots", { HARNESS_SLOTS: "65" }, "from 1 to 64"],
      ["negative slots", { HARNESS_SLOTS: "-1" }, "from 1 to 64"],
      ["fractional slots", { HARNESS_SLOTS: "2.5" }, "from 1 to 64"],
      ["word slots", { HARNESS_SLOTS: "many" }, "from 1 to 64"],
    ]
    for (const [name, override, message] of cases) {
      const result = parseEnv({ ...valid, ...override })
      expect(result.ok, name).toBe(false)
      expect(!result.ok && result.message, name).toContain(message)
    }
  })

  test("accepts the slot bounds", () => {
    for (const slots of ["1", "64"]) {
      expect(parseEnv({ ...valid, HARNESS_SLOTS: slots }).ok).toBe(true)
    }
  })

  test("an error message never repeats the token", () => {
    const result = parseEnv({ ...valid, HARNESS_SLOTS: "0" })
    expect(!result.ok && result.message).not.toContain("sob_secret")
  })

  test("the endpoint is the server url plus /mcp", () => {
    expect(mcpUrl(new URL("http://127.0.0.1:8080")).href).toBe(
      "http://127.0.0.1:8080/mcp"
    )
    expect(mcpUrl(new URL("https://obs.example.com/base/")).href).toBe(
      "https://obs.example.com/base/mcp"
    )
  })
})
