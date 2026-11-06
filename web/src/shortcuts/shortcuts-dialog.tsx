import { Fragment } from "react"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { modChordLabel } from "@/lib/shortcuts"

import { dashboardShortcuts, keyLabel } from "./chords"

type Row = { keys: readonly string[]; description: string }

function Keys({ keys }: { keys: readonly string[] }) {
  return (
    <span className="flex items-center gap-1">
      {keys.map((key, index) => (
        <Fragment key={index}>
          {index > 0 ? (
            <span className="text-xs text-muted-foreground">then</span>
          ) : null}
          <kbd className="min-w-6 rounded-sm border px-1.5 py-0.5 text-center text-xs">
            {keyLabel(key)}
          </kbd>
        </Fragment>
      ))}
    </span>
  )
}

export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const userAgent = navigator.userAgent
  const rows: Row[] = [
    ...dashboardShortcuts,
    {
      keys: [modChordLabel(userAgent, "O")],
      description: "Change how chart hover is shared",
    },
    {
      keys: [modChordLabel(userAgent, "K")],
      description: "Open the command palette",
    },
  ]
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>
            Panel keys act on the panel under the pointer, or the focused one.
          </DialogDescription>
        </DialogHeader>
        <dl className="grid grid-cols-[max-content_1fr] items-center gap-x-4 gap-y-2 text-[13px]">
          {rows.map((row) => (
            <Fragment key={row.description}>
              <dt>
                <Keys keys={row.keys} />
              </dt>
              <dd>{row.description}</dd>
            </Fragment>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  )
}
