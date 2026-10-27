import { useId, useState } from "react"
import { TriangleAlertIcon } from "lucide-react"

import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { filterLines, statusLabel } from "@/import/report"
import { countsText } from "@/import/record-summary"

import type { AgentGate } from "./agent-gate"
import { boxClass, EntryFrame } from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "import" }>
  scopePanelId: string | null
  gate: AgentGate
  onSelectPanel: (panelId: string) => void
  onAskToFix: (panelId: string, title: string) => void
}

export function ImportEntry({
  entry,
  scopePanelId,
  gate,
  onSelectPanel,
  onAskToFix,
}: Props) {
  const { record } = entry
  const reportId = useId()
  const [showReport, setShowReport] = useState(false)
  const unverified =
    scopePanelId === null
      ? entry.unverified
      : entry.unverified.filter((panel) => panel.panelId === scopePanelId)
  const lines = showReport ? filterLines(record.report, "all") : []

  return (
    <EntryFrame
      avatar={{ kind: "person", name: record.created_by.name }}
      who={record.created_by.name}
      verb="imported this dashboard from Grafana"
      at={entry.at}
    >
      <div className={cn(boxClass, "mt-1.5")}>
        <p>{countsText(record.report.counts)}</p>
        {unverified.length > 0 ? (
          <div className="mt-2 flex flex-col gap-1">
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <TriangleAlertIcon
                aria-hidden="true"
                className="size-3 flex-none text-status-warning"
              />
              Unverified in the latest version
            </p>
            <ul className="flex flex-col">
              {unverified.map((panel) => (
                <li
                  key={panel.panelId}
                  className="flex flex-col gap-0.5 py-1.5"
                >
                  <button
                    type="button"
                    className="self-start rounded-sm text-left font-medium wrap-anywhere underline decoration-border underline-offset-3 outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => onSelectPanel(panel.panelId)}
                  >
                    {panel.title}
                  </button>
                  <span className="text-xs wrap-anywhere text-muted-foreground">
                    {panel.reason}
                  </span>
                  {gate.kind === "allowed" ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="mt-0.5 -ml-2.5 self-start"
                      onClick={() => onAskToFix(panel.panelId, panel.title)}
                    >
                      Ask the agent to fix
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          </div>
        ) : record.report.counts.unverified > 0 && scopePanelId === null ? (
          <p className="mt-2 text-xs text-muted-foreground">
            No panel is unverified in the latest version.
          </p>
        ) : null}
        <button
          type="button"
          aria-expanded={showReport}
          aria-controls={reportId}
          className="mt-2 rounded-sm text-xs text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => setShowReport((open) => !open)}
        >
          {showReport ? "Hide the report" : "Show the report"}
        </button>
        <ul id={reportId} hidden={!showReport} className="divide-y">
          {lines.map((line) => {
            const status = statusLabel(line.status)
            return (
              <li key={line.key} className="flex flex-col gap-0.5 py-2">
                <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                  <StatusLabel tone={status.tone}>{status.text}</StatusLabel>
                  <span className="font-medium wrap-anywhere">
                    {line.title}
                  </span>
                </span>
                <span className="text-xs wrap-anywhere text-muted-foreground">
                  {line.reasons[0]}
                </span>
              </li>
            )
          })}
        </ul>
      </div>
    </EntryFrame>
  )
}
