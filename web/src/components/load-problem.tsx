import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

type Props = {
  title: string
  detail: string
  retrying: boolean
  onRetry: () => void
}

export function LoadProblem({ title, detail, retrying, onRetry }: Props) {
  return (
    <Alert variant="destructive">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <p>{detail}</p>
        <Button
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
