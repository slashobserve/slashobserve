import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

import { rowsOf } from "./builder"
import { withForm } from "./query-form"
import type { QueryProblem } from "./query-problem"
import { useLogsWorkspace } from "./workspace"

export const kqlReferenceUrl =
  "https://slashobserve.com/docs/use/logs-and-traces#kql-on-elasticsearch-and-opensearch"
const noModelText = "Add a model key to ask in plain language"

export function QueryProblemCard({
  problem,
  text,
  agentAvailable,
  onFixWithAsk,
}: {
  problem: QueryProblem
  text: string
  agentAvailable: boolean
  onFixWithAsk: (question: string) => void
}) {
  const { setDraft, run, setMode } = useLogsWorkspace()
  const { fix } = problem
  const builderText = fix?.text ?? text

  function apply(next: string) {
    setDraft(next)
    run(next)
  }

  return (
    <Alert variant="destructive">
      <AlertTitle>This query could not be read, so it did not run</AlertTitle>
      <AlertDescription className="flex flex-col items-start gap-2">
        <p>{problem.headline}</p>
        <p className="text-muted-foreground">{problem.explanation}</p>
        {problem.luceneSwitch ? (
          <p className="text-muted-foreground">
            KQL has no +, - or ~ operators. This reads as Lucene. Switching adds
            a lucene: prefix and runs the text as written. Filter pills stay.
          </p>
        ) : null}
        <div className="flex flex-wrap items-center gap-2">
          {fix === null ? null : (
            <Button type="button" size="xs" onClick={() => apply(fix.text)}>
              {fix.label}: {fix.text}
            </Button>
          )}
          {problem.luceneSwitch ? (
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => apply(withForm(text, "lucene"))}
            >
              Switch to Lucene
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            size="xs"
            aria-disabled={!agentAvailable || undefined}
            title={agentAvailable ? undefined : noModelText}
            onClick={() => {
              if (agentAvailable) onFixWithAsk(`Fix this KQL: ${text}`)
            }}
          >
            Fix it with Ask
          </Button>
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={rowsOf(builderText) === null}
            onClick={() => {
              setDraft(builderText)
              setMode("builder")
            }}
          >
            Open in the builder
          </Button>
          <a
            href={kqlReferenceUrl}
            target="_blank"
            rel="noreferrer"
            className="text-xs underline underline-offset-3"
          >
            KQL reference
          </a>
        </div>
      </AlertDescription>
    </Alert>
  )
}
