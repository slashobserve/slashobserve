import type { SyntaxProblem } from "./problems"

export function SyntaxProblemView({ problem }: { problem: SyntaxProblem }) {
  return (
    <div role="alert" className="flex flex-col gap-0.5">
      <p className="text-xs text-destructive">
        Line {problem.line}, column {problem.column}: {problem.message}
      </p>
      <pre className="m-0 overflow-x-auto font-mono text-xs">
        {problem.source}
        {"\n"}
        <span className="text-destructive">{problem.caret}</span>
      </pre>
      <p className="text-xs text-muted-foreground">
        Nothing ran. Fix the query and run it again.
      </p>
    </div>
  )
}
