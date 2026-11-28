import { AgentMark } from "@/components/agent-mark"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

import type { AskState } from "./use-ask"

export function AskAnswer({
  answer,
}: {
  answer: NonNullable<AskState["answer"]>
}) {
  return (
    <div className="flex flex-col gap-1">
      {answer.explanation !== "" ? (
        <p className="flex items-start gap-2 text-[13px] text-muted-foreground">
          <AgentMark size={20} className="text-primary" />
          <span className="min-w-0">{answer.explanation}</span>
        </p>
      ) : null}
      {answer.warnings.map((warning) => (
        <p key={warning} className="text-xs">
          <StatusLabel tone="warning">{warning}</StatusLabel>
        </p>
      ))}
    </div>
  )
}

export function AskProblem({
  problem,
}: {
  problem: NonNullable<AskState["problem"]>
}) {
  return (
    <Alert variant="destructive">
      <AlertDescription className="flex flex-col items-start gap-1">
        {problem.kind === "cannot_translate" ? (
          <>
            <p>I could not turn that into a query.</p>
            <p className="text-muted-foreground">{problem.reason}</p>
          </>
        ) : (
          <p>{problem.text}</p>
        )}
      </AlertDescription>
    </Alert>
  )
}

export function AskedFrom({
  question,
  onWriteAgain,
}: {
  question: string | undefined
  onWriteAgain: (() => void) | undefined
}) {
  return (
    <>
      {question !== undefined ? (
        <span className="min-w-0 truncate text-xs text-muted-foreground">
          From: {question}
        </span>
      ) : null}
      {onWriteAgain !== undefined ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="narrow:h-11"
          onClick={onWriteAgain}
        >
          Write again
        </Button>
      ) : null}
    </>
  )
}
