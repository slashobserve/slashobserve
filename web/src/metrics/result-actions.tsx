import { useRef, useState } from "react"
import { LayoutDashboardIcon, NotebookTextIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { AddPanelToDashboard } from "@/dashboards/add-panel-to-dashboard"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"

import { metricsPanelFor } from "./panel"
import type { MetricsMode } from "./search"

type Target = "dashboard" | "observation" | null

const notRunText = "Run a query first."

export function MetricsResultActions({
  uid,
  query,
  mode,
  range,
  observation,
}: {
  uid: string
  query: string | undefined
  mode: MetricsMode
  range: TimeRangeValue
  observation: string | undefined
}) {
  const dashboardTrigger = useRef<HTMLButtonElement>(null)
  const observationTrigger = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState<Target>(null)
  const disabled = query === undefined
  const hint = disabled ? notRunText : undefined
  const panelFor = (slot: { id: string; y: number; title: string }) =>
    metricsPanelFor({
      ...slot,
      datasource: uid,
      query: query ?? "",
      mode,
    })

  return (
    <>
      <Button
        ref={dashboardTrigger}
        type="button"
        variant="ghost"
        size="sm"
        className="narrow:h-11"
        disabled={disabled}
        title={hint}
        onClick={() => setOpen("dashboard")}
      >
        <LayoutDashboardIcon aria-hidden="true" />
        Add to dashboard
      </Button>
      <Button
        ref={observationTrigger}
        type="button"
        variant="outline"
        size="sm"
        className="narrow:h-11"
        disabled={disabled}
        title={hint}
        onClick={() => setOpen("observation")}
      >
        <NotebookTextIcon aria-hidden="true" />
        Add to observation
      </Button>
      {open === "dashboard" && !disabled ? (
        <AddPanelToDashboard
          idBase="metrics"
          defaultTitle="Metrics"
          description={
            mode === "table"
              ? "Adds a full-width table panel with this instant query under the dashboard's last row."
              : "Adds a full-width time series panel with this query under the dashboard's last row."
          }
          summary="Add a metrics panel from the metrics page"
          panelFor={panelFor}
          onClose={() => setOpen(null)}
          trigger={dashboardTrigger}
        />
      ) : null}
      {open === "observation" && !disabled ? (
        <AddToObservationPanel
          panel={panelFor({ id: "metrics", y: 0, title: "Metrics" })}
          from={range.from}
          to={range.to}
          {...(observation === undefined
            ? {}
            : { defaultObservationId: observation })}
          onClose={() => setOpen(null)}
          trigger={observationTrigger}
        />
      ) : null}
    </>
  )
}
