import { claimLoop } from "./claims"
import { loadEnv, mcpUrl } from "./env"
import { configureLog, log } from "./log"
import { ClaimClient, McpClient } from "./mcp"
import { createSession } from "./session/factory"
import { version } from "./version"

const shutdownGraceMs = 5_000

async function main(): Promise<void> {
  if (process.argv.includes("--version")) {
    process.stdout.write(`${version}\n`)
    return
  }
  const env = loadEnv()
  configureLog(env.logLevel)
  const base = new McpClient(mcpUrl(env.serverUrl), env.token)
  const controller = new AbortController()
  const shutdown = (): void => {
    controller.abort()
    setTimeout(() => process.exit(0), shutdownGraceMs).unref()
  }
  process.once("SIGTERM", shutdown)
  process.once("SIGINT", shutdown)
  log("info", "observe-agent started", { version, slots: env.slots })
  await claimLoop(
    new ClaimClient(base),
    env.slots,
    createSession,
    controller.signal
  )
  await base.close()
}

await main()
process.exit(0)
