import type { InboxReason } from "./inbox-item"
import { reasonLabel } from "./inbox-item"

export function ReasonChip({ reason }: { reason: InboxReason }) {
  return (
    <span className="inline-flex h-5 items-center rounded-chip bg-primary-container px-1.5 text-[11px] font-semibold text-on-primary-container">
      {reasonLabel(reason)}
    </span>
  )
}
