import { cn } from "@/lib/utils"

export function BarCell({
  text,
  widthCh,
  share,
  marker = null,
  critical = false,
}: {
  text: string
  widthCh: number
  share: number
  marker?: number | null
  critical?: boolean
}) {
  return (
    <div
      data-cell="bar"
      className="flex h-full w-full min-w-0 items-center gap-2.5"
    >
      <span
        className="shrink-0 overflow-hidden text-right whitespace-nowrap tabular-nums"
        style={{ width: `${widthCh}ch` }}
      >
        {text}
      </span>
      <div
        aria-hidden="true"
        className="relative h-1.5 min-w-10 flex-1 rounded-full bg-muted"
      >
        <div
          className={cn(
            "h-full rounded-full",
            critical ? "bg-status-critical" : "bg-series-1"
          )}
          style={{ width: `${share * 100}%` }}
        />
        {marker !== null ? (
          <div
            className="absolute inset-y-0 w-px -translate-x-1/2 bg-foreground"
            style={{ left: `${marker * 100}%` }}
          />
        ) : null}
      </div>
    </div>
  )
}
