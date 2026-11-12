import { describe, expect, test } from "bun:test"
import { createSession } from "../src/session/factory"
import { SessionError } from "../src/session/types"
import { fixtureDir } from "./harness"
import { settings, start } from "./sdk"

const live = new AbortController().signal

describe("createSession", () => {
  test("a provider it cannot call is a SessionError naming the provider", async () => {
    const failure = await createSession(
      settings({ provider: "gemini" }),
      start(),
      live
    ).catch((e: unknown) => e)
    expect(failure).toBeInstanceOf(SessionError)
    expect((failure as Error).message).toBe(
      "observe-agent cannot call provider gemini"
    )
  })

  test.each(["anthropic", "openai"])(
    "%s builds a session without calling the network",
    async (provider) => {
      const session = await createSession(settings({ provider }), start(), live)
      expect(typeof session.next).toBe("function")
      expect(typeof session.compact).toBe("function")
    }
  )

  test("replay is unchanged and reads its fixture directory", async () => {
    const session = await createSession(
      settings({ provider: "replay", replay_dir: fixtureDir }),
      start({ prompt: "create happy path" }),
      live
    )
    const answer = await session.next(undefined, {
      signal: live,
      onText: () => {},
    })
    expect(answer.turn.toolCalls.map((c) => c.name)).toEqual([
      "list_datasources",
      "list_metrics",
      "create_dashboard",
    ])
  })
})
