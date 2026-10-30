import { MaximizeIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"

import { jsonView } from "./cells/cell-model"

export function InspectCell({
  label,
  value,
  text,
}: {
  label: string
  value: unknown
  text: string
}) {
  const shown = jsonView(value)?.pretty ?? text
  return (
    <Dialog>
      <DialogTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className="shrink-0 text-muted-foreground"
            aria-label={`Inspect value of ${label}`}
          />
        }
      >
        <MaximizeIcon aria-hidden="true" />
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
        </DialogHeader>
        <pre className="max-h-96 overflow-auto font-mono text-xs break-words whitespace-pre-wrap">
          {shown}
        </pre>
      </DialogContent>
    </Dialog>
  )
}
