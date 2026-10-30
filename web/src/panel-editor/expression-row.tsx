import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { ExpressionQuery } from "@/spec"

type Props = {
  refName: string
  query: ExpressionQuery
  readOnly: boolean
  onRemove: () => void
}

export function expressionSummary(
  expression: ExpressionQuery["expression"]
): string {
  switch (expression.type) {
    case "math":
      return expression.math
    case "reduce":
      return `${expression.reducer} of ${expression.input}`
    case "resample":
      return `${expression.input} every ${expression.window}, ${expression.downsampler} down and ${expression.upsampler} up`
    case "threshold":
      return `${expression.input} ${expression.evaluator} ${expression.params.join(" and ")}`
  }
}

export function ExpressionRow({ refName, query, readOnly, onRemove }: Props) {
  return (
    <div
      role="group"
      aria-label={`Expression ${refName}`}
      className="flex flex-col gap-1.5"
    >
      <div className="flex min-h-6 flex-wrap items-center gap-x-2 gap-y-1 text-[13px]">
        <span className="font-medium">Expression {refName}</span>
        <span className="text-xs text-muted-foreground">
          {query.expression.type}
          {query.hide ? ", hidden" : ""}
        </span>
        {readOnly ? null : (
          <Button
            variant="ghost"
            size="icon-xs"
            className="ml-auto"
            aria-label={`Remove expression ${refName}`}
            onClick={onRemove}
          >
            <XIcon aria-hidden="true" />
          </Button>
        )}
      </div>
      <p className="rounded-control border border-border px-2 py-1.5 font-mono text-[13px] break-words text-muted-foreground">
        {expressionSummary(query.expression)}
      </p>
    </div>
  )
}
