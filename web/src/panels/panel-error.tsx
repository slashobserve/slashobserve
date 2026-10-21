import { CircleAlertIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown error"
}

export function PanelErrorState({
  title,
  error,
  onRetry,
}: {
  title: string
  error: unknown
  onRetry?: () => void
}) {
  const message = errorMessage(error)
  return (
    <div className="flex size-full flex-col items-center justify-center gap-2 p-3 text-center">
      <CircleAlertIcon className="size-5 text-destructive" />
      <p className="text-sm font-medium">{title}</p>
      {message.toLowerCase() !== title.toLowerCase() ? (
        <p className="line-clamp-3 text-xs text-muted-foreground">{message}</p>
      ) : null}
      <Button variant="outline" size="sm" onClick={onRetry}>
        Retry
      </Button>
    </div>
  )
}
