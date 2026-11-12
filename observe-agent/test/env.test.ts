import { describe, expect, test } from "bun:test"
import { mcpUrl, parseEnv } from "../src/env"

const valid = {
  OBSERVE_AGENT_SERVER_URL: "http://127.0.0.1:8080",
  OBSERVE_AGENT_TOKEN: "soa_secret",
  OBSERVE_AGENT_SLOTS: "4",
  OBSERVE_AGENT_LOG_LEVEL: "debug",
}

describe("env", () => {
  test("reads the four variables", () => {
    const result = parseEnv(valid)
    expect(result.ok).toBe(true)
    if (!result.ok) {
      return
    }
    expect(result.env.serverUrl.href).toBe("http://127.0.0.1:8080/")
    expect(result.env.token).toBe("soa_secret")
    expect(result.env.slots).toBe(4)
    expect(result.env.logLevel).toBe("debug")
  })

  test("slots default to one and the level to info", () => {
    const result = parseEnv({
      OBSERVE_AGENT_SERVER_URL: valid.OBSERVE_AGENT_SERVER_URL,
      OBSERVE_AGENT_TOKEN: valid.OBSERVE_AGENT_TOKEN,
    })
    expect(result.ok && result.env.slots).toBe(1)
    expect(result.ok && result.env.logLevel).toBe("info")
  })

  test("refuses what the server would never send", () => {
    const cases: Array<[string, Record<string, string>, string]> = [
      [
        "missing url",
        { OBSERVE_AGENT_SERVER_URL: "" },
        "SERVER_URL is not set",
      ],
      [
        "url without a scheme",
        { OBSERVE_AGENT_SERVER_URL: "127.0.0.1:8080" },
        "http or https",
      ],
      [
        "non http scheme",
        { OBSERVE_AGENT_SERVER_URL: "ftp://example.com" },
        "http or https",
      ],
      ["missing token", { OBSERVE_AGENT_TOKEN: " " }, "TOKEN is not set"],
      ["zero slots", { OBSERVE_AGENT_SLOTS: "0" }, "from 1 to 64"],
      ["too many slots", { OBSERVE_AGENT_SLOTS: "65" }, "from 1 to 64"],
      ["negative slots", { OBSERVE_AGENT_SLOTS: "-1" }, "from 1 to 64"],
      ["fractional slots", { OBSERVE_AGENT_SLOTS: "2.5" }, "from 1 to 64"],
      ["word slots", { OBSERVE_AGENT_SLOTS: "many" }, "from 1 to 64"],
    ]
    for (const [name, override, message] of cases) {
      const result = parseEnv({ ...valid, ...override })
      expect(result.ok, name).toBe(false)
      expect(!result.ok && result.message, name).toContain(message)
    }
  })

  test("accepts the slot bounds", () => {
    for (const slots of ["1", "64"]) {
      expect(parseEnv({ ...valid, OBSERVE_AGENT_SLOTS: slots }).ok).toBe(true)
    }
  })

  test("an error message never repeats the token", () => {
    const result = parseEnv({ ...valid, OBSERVE_AGENT_SLOTS: "0" })
    expect(!result.ok && result.message).not.toContain("soa_secret")
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
