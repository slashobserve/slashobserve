import { zoneLabel } from "@/lib/date-format"
import { zonedDateTime } from "@/lib/time"

import type { PanelPayload } from "./payload"

export function rangeCaption(
  payload: PanelPayload,
  zone: string | undefined
): string {
  const from = payload.data.meta.from ?? payload.from
  const to = payload.data.meta.to ?? payload.to
  const end = Date.parse(to)
  if (Number.isNaN(Date.parse(from)) || Number.isNaN(end)) {
    return `${payload.from} to ${payload.to}`
  }
  return `${zonedDateTime(from, zone)} to ${zonedDateTime(to, zone)} ${zone ?? zoneLabel(end)}`
}
