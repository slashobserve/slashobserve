import { ApiError } from "@/api/errors"
import { loadProblemText } from "@/service-accounts/queries"

import type { ActivityType } from "./event-types"

export type WebhookInput = { name: string; url: string; events: ActivityType[] }
export type WebhookProblems = Partial<Record<keyof WebhookInput, string>>

const maxNameCharacters = 100
const maxURLBytes = 2048

export function trimWebhook(input: WebhookInput): WebhookInput {
  return { ...input, name: input.name.trim(), url: input.url.trim() }
}

function urlProblem(url: string): string | undefined {
  if (url === "") return "Enter the address that receives the events."
  if (new TextEncoder().encode(url).length > maxURLBytes) {
    return `The address can be at most ${maxURLBytes} characters.`
  }
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return "Enter a full https address, such as https://hooks.example.com/slashobserve."
  }
  if (parsed.protocol !== "https:")
    return "The address must start with https://."
  if (parsed.port !== "") return "The address must use port 443."
  if (parsed.username !== "" || parsed.password !== "") {
    return "The address cannot hold a user name or password."
  }
  if (parsed.hash !== "") return "The address cannot end with a # fragment."
  return undefined
}

export function webhookProblems(raw: WebhookInput): WebhookProblems {
  const input = trimWebhook(raw)
  const problems: WebhookProblems = {}
  if (input.name === "") problems.name = "Enter a name."
  else if ([...input.name].length > maxNameCharacters) {
    problems.name = `The name can be at most ${maxNameCharacters} characters.`
  }
  const url = urlProblem(input.url)
  if (url !== undefined) problems.url = url
  if (input.events.length === 0) problems.events = "Choose at least one event."
  return problems
}

export function isDomainNotAllowed(error: unknown): boolean {
  return (
    error instanceof ApiError && error.code === "outbound_domain_not_allowed"
  )
}

export function webhookProblemText(error: unknown): string {
  if (!(error instanceof ApiError)) return loadProblemText(error)
  switch (error.code) {
    case "secret_key_missing":
      return "Set SLASHOBSERVE_SECRET_KEY on the server and restart."
    case "secret_unreadable":
      return "The signing secret cannot be read with the current SLASHOBSERVE_SECRET_KEY. Rotate the secret, then enable the webhook."
    case "too_many_webhooks":
      return "At most 20 webhooks can exist. Delete one first."
  }
  return error.message
}

const disabledReasons: Record<string, string> = {
  manual: "Turned off by an Admin",
  gone: "The receiver answered 410 Gone",
  failing: "50 attempts in a row failed",
  owner: "Its owner is gone or no longer an Admin",
  secret_unreadable: "The signing secret cannot be read; rotate it",
}

export function disabledReasonText(reason: string | null): string {
  if (reason === null) return "Disabled"
  return disabledReasons[reason] ?? "Disabled"
}
