import { pillHue, pillLabels } from "./cell-model"

export function PillCell({ value }: { value: unknown }) {
  const labels = pillLabels(value)
  if (labels.length === 0) {
    return <span className="text-muted-foreground">-</span>
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      {labels.map((label) => (
        <span
          key={label}
          data-pill=""
          className="inline-flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5"
        >
          <span
            aria-hidden="true"
            className="size-1.5 shrink-0 rounded-full"
            style={{ backgroundColor: `var(--series-${pillHue(label)})` }}
          />
          {label}
        </span>
      ))}
    </span>
  )
}
