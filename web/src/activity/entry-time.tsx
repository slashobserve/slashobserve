import { useNow } from "@/components/relative-time"
import { formatDate } from "@/lib/date-format"
import { formatDateTime, isSubMinuteOffset } from "@/lib/time"

export function EntryTime({ iso }: { iso: string }) {
  const now = useNow()
  const ms = Date.parse(iso)
  const label = Number.isNaN(ms)
    ? iso
    : isSubMinuteOffset(iso, now)
      ? "Just now"
      : formatDate(ms, "HH:mm")
  return (
    <time dateTime={iso} title={formatDateTime(iso)}>
      {label}
    </time>
  )
}
