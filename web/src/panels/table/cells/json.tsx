import { jsonView } from "./cell-model"

export function JsonCell({ value, text }: { value: unknown; text: string }) {
  const view = jsonView(value)
  if (!view) return <>{text}</>
  return (
    <details>
      <summary className="cursor-pointer rounded-sm font-mono outline-none focus-visible:ring-2 focus-visible:ring-ring">
        {view.summary}
      </summary>
      <pre className="mt-1 max-h-60 overflow-auto font-mono text-xs">
        {view.pretty}
      </pre>
    </details>
  )
}
