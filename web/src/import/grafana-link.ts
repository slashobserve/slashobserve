import { api } from "@/api/client"
import { ApiError, unwrap, sentence } from "@/api/errors"

export const adminOnlyLinkText =
  "Only Admins can import from a link, through a Grafana connection. Paste the dashboard's JSON instead."

const unavailableText = "Grafana did not answer. Try again, or paste the JSON."
const failedText =
  "Could not fetch the dashboard from that link. Try again, or paste the JSON."

export function looksLikeLink(text: string): boolean {
  const trimmed = text.trim()
  return (
    /^https?:\/\/\S+$/i.test(trimmed) &&
    !trimmed.includes("{") &&
    !trimmed.includes("\n")
  )
}

export function linkFailureText(error: unknown): string {
  if (!(error instanceof ApiError)) return failedText
  if (error.status === 403 && error.code === "forbidden_role") {
    return adminOnlyLinkText
  }
  if (error.code === "grafana_unavailable") return unavailableText
  const shown = [
    "no_connection",
    "not_a_dashboard_link",
    "dashboard_not_found",
    "outbound_domain_not_allowed",
  ]
  return shown.includes(error.code) ? sentence(error.message) : failedText
}

export function fetchGrafanaLink(url: string) {
  return api.POST("/grafana/link", { body: { url } }).then(unwrap)
}
