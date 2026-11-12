import { type Level, parseLevel } from "./log"

export type Env = {
  serverUrl: URL
  token: string
  slots: number
  logLevel: Level
}

export type EnvResult = { ok: true; env: Env } | { ok: false; message: string }

export const exitUsage = 2
export const minSlots = 1
export const maxSlots = 64

type Source = Readonly<Record<string, string | undefined>>

function parseServerUrl(text: string): URL | undefined {
  try {
    const url = new URL(text)
    return url.protocol === "http:" || url.protocol === "https:"
      ? url
      : undefined
  } catch {
    return undefined
  }
}

export function parseEnv(source: Source): EnvResult {
  const serverText = source["OBSERVE_AGENT_SERVER_URL"]?.trim() ?? ""
  if (serverText === "") {
    return { ok: false, message: "OBSERVE_AGENT_SERVER_URL is not set" }
  }
  const serverUrl = parseServerUrl(serverText)
  if (serverUrl === undefined) {
    return {
      ok: false,
      message: "OBSERVE_AGENT_SERVER_URL is not an http or https URL",
    }
  }
  const token = source["OBSERVE_AGENT_TOKEN"]?.trim() ?? ""
  if (token === "") {
    return { ok: false, message: "OBSERVE_AGENT_TOKEN is not set" }
  }
  const slotsText = source["OBSERVE_AGENT_SLOTS"]?.trim() ?? "1"
  const slots = /^\d+$/.test(slotsText) ? Number(slotsText) : 0
  if (slots < minSlots || slots > maxSlots) {
    return {
      ok: false,
      message: `OBSERVE_AGENT_SLOTS must be a whole number from ${minSlots} to ${maxSlots}`,
    }
  }
  return {
    ok: true,
    env: {
      serverUrl,
      token,
      slots,
      logLevel: parseLevel(source["OBSERVE_AGENT_LOG_LEVEL"]) ?? "info",
    },
  }
}

export function loadEnv(): Env {
  const result = parseEnv(process.env)
  if (!result.ok) {
    process.stderr.write(`observe-agent: ${result.message}\n`)
    process.exit(exitUsage)
  }
  return result.env
}

export function mcpUrl(serverUrl: URL): URL {
  const base = serverUrl.href.replace(/\/+$/, "")
  return new URL(`${base}/mcp`)
}
