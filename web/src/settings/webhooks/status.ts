import type { StatusTone } from "@/components/status-label"

import { disabledReasonText } from "./webhook-problems"
import type { Webhook, WebhookDelivery } from "./queries"

export type WebhookStatus = {
  tone: StatusTone
  label: string
  detail: string | null
  waiting: string | null
}

function waitingText(waiting: Webhook["waiting_events"]): string | null {
  if (waiting === null || waiting.count === 0) return null
  if (waiting.more) return `${waiting.count} or more events waiting`
  return `${waiting.count} ${waiting.count === 1 ? "event" : "events"} waiting`
}

export function webhookStatus(
  webhook: Pick<
    Webhook,
    "state" | "disabled_reason" | "consecutive_failures" | "waiting_events"
  >
): WebhookStatus {
  switch (webhook.state) {
    case "disabled":
      return {
        tone: webhook.disabled_reason === "manual" ? "none" : "critical",
        label: "Disabled",
        detail: disabledReasonText(webhook.disabled_reason),
        waiting: waitingText(webhook.waiting_events),
      }
    case "retrying": {
      const n = webhook.consecutive_failures
      return {
        tone: "warning",
        label: "Retrying",
        detail: `${n} ${n === 1 ? "attempt" : "attempts"} in a row failed`,
        waiting: waitingText(webhook.waiting_events),
      }
    }
    case "sending":
      return { tone: "ok", label: "Sending", detail: null, waiting: null }
  }
}

const errorTexts: Record<string, string> = {
  status: "The receiver answered with an error status",
  unreachable: "The receiver could not be reached",
  blocked_address: "The address resolves to a private address",
  domain_not_allowed: "The domain is not on the outbound allowlist",
  too_large: "The event is larger than 64 KiB",
  skipped_by_admin: "An Admin skipped this event",
  expired: "The event was more than 24 hours old",
}

export function errorText(code: string | null): string {
  if (code === null) return "-"
  return errorTexts[code] ?? code
}

export function outcomeTone(outcome: WebhookDelivery["outcome"]): StatusTone {
  switch (outcome) {
    case "delivered":
      return "ok"
    case "skipped":
      return "warning"
    case "failed":
      return "critical"
  }
}

export function deliverySummary(
  delivery: Pick<
    WebhookDelivery,
    "outcome" | "status_code" | "duration_ms" | "error_code"
  >
): string {
  const status =
    delivery.status_code === null ? "" : `status ${delivery.status_code}, `
  const took = `${delivery.duration_ms} ms`
  if (delivery.outcome === "delivered") return `Delivered: ${status}${took}`
  return `Not delivered: ${errorText(delivery.error_code)} (${status}${took})`
}
