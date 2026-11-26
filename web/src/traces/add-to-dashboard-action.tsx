import { useRef, useState } from "react"
import { LayoutDashboardIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { AddPanelToDashboard } from "@/dashboards/add-panel-to-dashboard"
import type { DataPanel } from "@/spec"

import { placedPanel } from "./trace-panel"

export function AddToDashboardAction({
  panel,
  size = "sm",
}: {
  panel: DataPanel
  size?: "sm" | "default"
}) {
  const trigger = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const kind = panel.type === "table" ? "table" : "traces"
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
        <LayoutDashboardIcon aria-hidden="true" />
        <span className="@max-[1100px]/top:sr-only">Add to dashboard</span>
      </Button>
      {open ? (
        <AddPanelToDashboard
          idBase={panel.id}
          defaultTitle={panel.title ?? "Traces"}
          description={`Adds a full-width ${kind} panel with this query under the dashboard's last row.`}
          summary={`Add a ${kind} panel from the traces page`}
          panelFor={(slot) => placedPanel(panel, slot)}
          onClose={() => setOpen(false)}
          trigger={trigger}
        />
      ) : null}
    </>
  )
}
