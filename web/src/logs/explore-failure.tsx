import { RotateCwIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"

export function ExploreFailure({
  text,
  onRetry,
}: {
  text: string
  onRetry: () => void
}) {
  return (
    <div
      role="alert"
      className="flex items-center gap-3 rounded-card border p-4"
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
