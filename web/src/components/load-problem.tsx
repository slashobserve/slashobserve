import type { Ref } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

type Props = {
  title: string
  detail: string
  retrying: boolean
  onRetry: () => void
  retryRef?: Ref<HTMLButtonElement>
}

export function LoadProblem({
  title,
  detail,
  retrying,
  onRetry,
  retryRef,
}: Props) {
  return (
    <Alert variant="destructive">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <p>{detail}</p>
        <Button
          ref={retryRef}
          variant="outline"
          size="sm"
          className="text-foreground"
          disabled={retrying}
          focusableWhenDisabled
          aria-busy={retrying}
          onClick={onRetry}
        >
          Retry
        </Button>
      </AlertDescription>
    </Alert>
  )
}
