import { Fragment } from "react"

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { modChordLabel } from "@/lib/shortcuts"

export function logsShortcuts(
  userAgent: string
): { keys: string; description: string }[] {
  return [
    { keys: "/", description: "Focus the query" },
    { keys: `${modChordLabel(userAgent, "")}Enter`, description: "Run" },
    { keys: modChordLabel(userAgent, "I"), description: "Switch mode" },
    { keys: "Ctrl Space", description: "Complete" },
    { keys: "j / k", description: "Move to the next or previous line" },
    { keys: "Enter", description: "Open the line" },
    { keys: "Esc", description: "Close the line" },
    { keys: "?", description: "This list" },
  ]
}

export function LogsShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-label="Keyboard shortcuts" className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>
            Letter keys work when the cursor is not in a text field.
          </DialogDescription>
        </DialogHeader>
        <dl className="grid grid-cols-[max-content_1fr] items-center gap-x-4 gap-y-2 text-[13px]">
          {logsShortcuts(navigator.userAgent).map((row) => (
            <Fragment key={row.description}>
              <dt>
                <kbd className="rounded-sm border px-1.5 py-0.5 text-center text-xs">
                  {row.keys}
                </kbd>
              </dt>
              <dd>{row.description}</dd>
            </Fragment>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  )
}
