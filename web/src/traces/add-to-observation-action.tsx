import { useRef, useState } from "react"
import { NotebookTextIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import { useObservationOrigin } from "@/observations/origin"
import type { DataPanel } from "@/spec"

export function AddToObservationAction({
  panel,
  range,
  size = "sm",
  labelClassName,
}: {
  panel: DataPanel
  range: { from: string; to: string }
  size?: "sm" | "default"
  labelClassName?: string
}) {
  const trigger = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const observation = useObservationOrigin()
  return (
    <>
      <Button
        ref={trigger}
        type="button"
        variant="outline"
        size={size}
        className="narrow:h-11"
        onClick={() => setOpen(true)}
      >
        <NotebookTextIcon aria-hidden="true" />
        <span className={labelClassName}>Add to observation</span>
      </Button>
      {open ? (
        <AddToObservationPanel
          panel={panel}
          from={range.from}
          to={range.to}
          onClose={() => setOpen(false)}
          trigger={trigger}
          defaultObservationId={observation}
        />
      ) : null}
    </>
  )
}
