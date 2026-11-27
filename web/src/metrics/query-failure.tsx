import { useQuery } from "@tanstack/react-query"

import { ApiError } from "@/api/errors"
import { datasourceKeys } from "@/api/keys"
import { exploreErrorText } from "@/logs/explore"
import { ExploreFailure } from "@/logs/explore-failure"
import { lintQuery } from "@/query-editor/server-lint"

import { failureHint, syntaxProblem } from "./problems"
import { SyntaxProblemView } from "./syntax-problem"

const rejectedQueryCode = "query_invalid"

export function QueryFailure({
  uid,
  query,
  error,
  onRetry,
}: {
  uid: string
  query: string
  error: unknown
  onRetry: () => void
}) {
  const rejected = error instanceof ApiError && error.code === rejectedQueryCode
  const lint = useQuery({
    queryKey: datasourceKeys.lint(uid, query),
    queryFn: ({ signal }) => lintQuery(uid, query, signal),
    enabled: rejected,
    staleTime: 60_000,
  })
  const problem = rejected ? syntaxProblem(lint.data ?? null, query) : null
  if (problem !== null) return <SyntaxProblemView problem={problem} />
  const hint = failureHint(error)
  return (
    <>
      <ExploreFailure text={exploreErrorText(error)} onRetry={onRetry} />
      {hint === null ? null : (
        <p className="text-[13px] text-muted-foreground">{hint}</p>
      )}
    </>
  )
}
