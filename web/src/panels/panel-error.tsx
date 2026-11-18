import { CircleAlertIcon } from "lucide-react"

import { ApiError, failedQueryText } from "@/api/errors"
import { Button } from "@/components/ui/button"

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return failedQueryText(error)
  return error instanceof Error ? error.message : "Unknown error"
}

export function PanelErrorState({
  title,
  error,
  message = errorMessage(error),
  onRetry,
  retryLabel = "Retry",
}: {
  title: string
  error?: unknown
  message?: string
  onRetry?: () => void
  retryLabel?: string
}) {
  return (
    <div className="flex size-full flex-col items-center justify-center gap-2 p-3 text-center">
      <CircleAlertIcon className="size-5 text-destructive" />
      <p className="text-sm font-medium">{title}</p>
      {message.toLowerCase() !== title.toLowerCase() ? (
        <p
          title={message}
          className="line-clamp-4 text-xs text-muted-foreground"
        >
          {message}
        </p>
      ) : null}
      {onRetry ? (
        <Button variant="outline" size="sm" onClick={onRetry}>
          {retryLabel}
        </Button>
      ) : null}
    </div>
  )
}
