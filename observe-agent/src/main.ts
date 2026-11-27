import { claimLoop } from "./claims"
import { loadEnv, mcpUrl } from "./env"
import { configureLog, log } from "./log"
import { ClaimClient, McpClient } from "./mcp"
import { createSession } from "./session/factory"
import { exitCodes, shutdownGraceMs, shutdownOutcome } from "./shutdown"
import { version } from "./version"

async function main(): Promise<number> {
  if (process.argv.includes("--version")) {
    process.stdout.write(`${version}\n`)
    return 0
  }
  const env = loadEnv()
  configureLog(env.logLevel)
  const base = new McpClient(mcpUrl(env.serverUrl), env.token)
  const controller = new AbortController()
  const stop = (): void => controller.abort()
  process.on("SIGTERM", stop)
  process.on("SIGINT", stop)
  log("info", "observe-agent started", { version, slots: env.slots })
  const work = claimLoop(
    new ClaimClient(base),
    env.slots,
    createSession,
    controller.signal
  ).then(() => base.close())
  const outcome = await shutdownOutcome(
    work,
    controller.signal,
    shutdownGraceMs
  )
  log(outcome === "forced" ? "warn" : "info", "observe-agent stopped", {
    shutdown: outcome,
  })
  return exitCodes[outcome]
}

process.exit(await main())
