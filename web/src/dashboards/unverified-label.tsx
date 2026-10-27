import { StatusLabel } from "@/components/status-label"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

export function UnverifiedLabel({
  title,
  reason,
}: {
  title: string
  reason: string
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <button
            type="button"
            aria-label={`${title} is unverified: ${reason}`}
            className="shrink-0 rounded-control px-1 text-xs whitespace-nowrap text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-unverified=""
          />
        }
      >
        <StatusLabel tone="critical">Unverified</StatusLabel>
      </TooltipTrigger>
      <TooltipContent>
        Its queries failed when this version was saved: {reason}
      </TooltipContent>
    </Tooltip>
  )
}
