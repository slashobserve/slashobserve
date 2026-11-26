import { RotateCwIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"

export function ExploreFailure({
  text,
  onRetry,
  echo = false,
}: {
  text: string
  onRetry: () => void
  echo?: boolean
}) {
  return (
    <div
      role={echo ? undefined : "alert"}
      className={
        echo
          ? "flex flex-wrap items-center gap-x-3 gap-y-2"
          : "flex items-center gap-3 rounded-card border p-4"
      }
    >
      <StatusLabel tone="critical">Failed</StatusLabel>
      <p className="min-w-0 flex-1 text-[13px]">{text}</p>
      <Button type="button" variant="outline" size="xs" onClick={onRetry}>
        <RotateCwIcon aria-hidden="true" />
        Retry
      </Button>
    </div>
  )
}
