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

export function LiveIndicator({
  status,
  pinned,
}: {
  status: LiveStatus
  pinned: boolean
}) {
  return (
    <div role="status" className="flex min-w-24 justify-start sm:justify-end">
      <Tooltip disableHoverablePopup>
        <TooltipTrigger
          render={
            <button
              type="button"
              className="-mx-1.5 flex items-center gap-1.5 rounded-md border border-transparent px-1.5 py-1 text-xs text-muted-foreground outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
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
        <TooltipContent side="bottom" align="end" sideOffset={8}>
          {pinned
            ? "Pinned to this version. Newer ones appear as a notice."
            : "New versions appear here as they are saved."}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
