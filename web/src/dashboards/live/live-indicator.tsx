import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

import type { LiveStatus } from "./use-dashboard-events"

const labels: Record<LiveStatus, string> = {
  live: "Live",
  connecting: "Connecting",
  reconnecting: "Reconnecting",
}

export function LiveIndicator({ status }: { status: LiveStatus }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            role="status"
            className="flex min-w-24 items-center justify-start gap-1.5 text-xs text-muted-foreground sm:justify-end"
          />
        }
      >
        <span
          aria-hidden="true"
          className={cn(
            "size-2 shrink-0 rounded-full",
            status === "live"
              ? "bg-threshold-ok"
              : "bg-muted-foreground motion-safe:animate-pulse"
          )}
        />
        {labels[status]}
      </TooltipTrigger>
      <TooltipContent side="bottom" align="end">
        Updates appear automatically when a new version is saved.
      </TooltipContent>
    </Tooltip>
  )
}
