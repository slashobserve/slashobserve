import type { MarkersLegend } from "./geo-model"

export function LayerLegend({ legends }: { legends: MarkersLegend[] }) {
  return (
    <div
      role="group"
      aria-label="Map legend"
      className="pointer-events-none absolute right-2 bottom-6 flex max-h-[60%] max-w-[50%] flex-col gap-1.5 overflow-hidden rounded-control border border-border bg-background px-2 py-1 text-xs"
    >
      {legends.map((legend, index) => (
        <div key={index} className="flex flex-col gap-0.5">
          <span className="truncate font-medium text-foreground">
            {legend.name}
          </span>
          {legend.entries.map((entry, entryIndex) => (
            <span
              key={entryIndex}
              className="flex items-center gap-1.5 text-muted-foreground"
            >
              <span
                aria-hidden="true"
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: entry.color }}
              />
              <span className="truncate tabular-nums">{entry.text}</span>
            </span>
          ))}
        </div>
      ))}
    </div>
  )
}
