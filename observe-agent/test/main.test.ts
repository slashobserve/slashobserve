import { expect, test } from "bun:test"
import { join } from "node:path"
import { exitCodes, shutdownGraceMs } from "../src/shutdown"

async function linesUntil(
  stream: ReadableStream<Uint8Array>,
  seen: string[],
  done: (line: string) => boolean
): Promise<void> {
  const decoder = new TextDecoder()
  let rest = ""
  for await (const chunk of stream) {
    const parts = (rest + decoder.decode(chunk, { stream: true })).split("\n")
    rest = parts.pop() ?? ""
    seen.push(...parts)
    if (parts.some(done)) {
      return
    }
  }
}

test(
  "a second stop signal during the drain still ends with the stopped line and the forced exit code",
  async () => {
    const neverAnswers = Bun.listen({
      hostname: "127.0.0.1",
      port: 0,
      socket: { data() {}, open() {} },
    })
    const child = Bun.spawn(["bun", join(import.meta.dir, "../src/main.ts")], {
      env: {
        PATH: process.env.PATH ?? "",
        OBSERVE_AGENT_SERVER_URL: `http://127.0.0.1:${neverAnswers.port}`,
        OBSERVE_AGENT_TOKEN: "sob_test",
        OBSERVE_AGENT_SLOTS: "1",
      },
      stderr: "pipe",
      stdout: "ignore",
    })
    try {
      const lines: string[] = []
      const stopped = (line: string): boolean =>
        line.includes("observe-agent stopped")
      const reader = linesUntil(child.stderr, lines, stopped)
      while (!lines.some((line) => line.includes("observe-agent started"))) {
        await Bun.sleep(20)
      }
      await Bun.sleep(200)
      child.kill("SIGTERM")
      await Bun.sleep(200)
      child.kill("SIGTERM")
      const code = await child.exited
      await reader
      expect(code).toBe(exitCodes.forced)
      expect(JSON.parse(lines.find(stopped) ?? "{}")).toMatchObject({
        msg: "observe-agent stopped",
        shutdown: "forced",
      })
    } finally {
      child.kill("SIGKILL")
      neverAnswers.stop(true)
    }
  },
  shutdownGraceMs + 10_000
)
