import type { StatusTone } from "@/components/status-label"

import { disabledReasonText } from "./webhook-problems"
import type { Webhook, WebhookDelivery } from "./queries"

export type WebhookStatus = {
  tone: StatusTone
  label: string
  detail: string | null
}

export function webhookStatus(
  webhook: Pick<Webhook, "state" | "disabled_reason" | "consecutive_failures">
): WebhookStatus {
  switch (webhook.state) {
    case "disabled":
      return {
        tone: "critical",
        label: "Disabled",
        detail: disabledReasonText(webhook.disabled_reason),
      }
    case "retrying": {
      const n = webhook.consecutive_failures
      return {
        tone: "warning",
        label: "Retrying",
        detail: `${n} ${n === 1 ? "attempt" : "attempts"} in a row failed`,
      }
    }
    case "sending":
      return { tone: "ok", label: "Sending", detail: null }
  }
}

const errorTexts: Record<string, string> = {
  status: "The receiver answered with an error status",
  unreachable: "The receiver could not be reached",
  blocked_address: "The address resolves to a private address",
  domain_not_allowed: "The domain is not on the outbound allowlist",
  too_large: "The event is larger than 64 KiB",
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
