import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

export type ContextChip = { id: string; label: string; onRemove?: () => void }

export function ContextChips({ chips }: { chips: ContextChip[] }) {
  if (chips.length === 0) return null
  return (
    <ul aria-label="Context" className="flex flex-wrap gap-1.5">
      {chips.map((chip) => (
        <li
          key={chip.id}
          className="inline-flex h-6 max-w-full items-center gap-1 rounded-control border px-2 text-xs"
        >
          <span className="truncate">{chip.label}</span>
          {chip.onRemove !== undefined ? (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="-mr-1.5 size-5"
              aria-label={`Remove ${chip.label}`}
              onClick={chip.onRemove}
            >
              <XIcon />
            </Button>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
