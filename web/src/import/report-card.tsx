import { useId, useRef, useState, type KeyboardEvent } from "react"
import { Link } from "@tanstack/react-router"

import { StatusLabel } from "@/components/status-label"
import { importedFilter } from "@/health/health-labels"
import { cn } from "@/lib/utils"

import { BlockList } from "./block-list"
import {
  filterCounts,
  filterLines,
  initialFilter,
  reportFilters,
  statusLabel,
  type ImportReport,
  type ReportFilter,
} from "./report"

type Props = {
  report: ImportReport
  imported: boolean
  busy: boolean
}

const queriesRunHint =
  "Queries run when you import. A panel whose queries fail is kept and marked unverified, and you can ask the agent to fix it from the dashboard."
const fixUnverifiedHint =
  "Ask the agent to fix the unverified panels from the dashboard's activity panel."

export function ReportCard({ report, imported, busy }: Props) {
  const [filter, setFilter] = useState<ReportFilter>(() =>
    initialFilter(report)
  )
  const id = useId()
  const [showBlocks, setShowBlocks] = useState(false)
  const hasBlocks = (report.blocks ?? []).length > 0
  const tabs = useRef<Record<string, HTMLButtonElement | null>>({})
  const counts = filterCounts(report)
  const lines = filterLines(report, filter)
  const waitsInHealth =
    imported && (report.counts.placeholder > 0 || report.counts.unverified > 0)
  const closingHint =
    imported && report.counts.unverified > 0
      ? fixUnverifiedHint
      : queriesRunHint

  const move = (event: KeyboardEvent, index: number) => {
    const last = reportFilters.length - 1
    const next =
      event.key === "ArrowRight"
        ? (index + 1) % reportFilters.length
        : event.key === "ArrowLeft"
          ? (index + last) % reportFilters.length
          : event.key === "Home"
            ? 0
            : event.key === "End"
              ? last
              : null
    if (next === null) return
    event.preventDefault()
    const target = reportFilters[next].id
    setFilter(target)
    tabs.current[target]?.focus()
  }

  return (
    <section
      aria-labelledby={`${id}-title`}
      aria-busy={busy || undefined}
      className="rounded-card border bg-background"
    >
      <div className="flex flex-wrap items-center justify-between gap-2.5 border-b px-3.5 py-3">
        <h2 id={`${id}-title`} className="text-sm font-semibold">
          Import report
        </h2>
        <div
          role="tablist"
          aria-label="Filter panels"
          className="flex flex-wrap gap-3.5"
        >
          {reportFilters.map((item, index) => {
            const selected = item.id === filter
            return (
              <button
                key={item.id}
                ref={(element) => {
                  tabs.current[item.id] = element
                }}
                type="button"
                role="tab"
                id={`${id}-tab-${item.id}`}
                aria-selected={selected}
                aria-controls={`${id}-list`}
                tabIndex={selected ? 0 : -1}
                onClick={() => setFilter(item.id)}
                onKeyDown={(event) => move(event, index)}
                className={cn(
                  "rounded-control border-0 border-b-2 border-transparent py-0.5 text-[13px] text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  selected && "border-foreground text-foreground"
                )}
              >
                {item.label}{" "}
                <span className="text-xs text-muted-foreground">
                  {counts[item.id]}
                </span>
              </button>
            )
          })}
        </div>
      </div>
      <div
        id={`${id}-list`}
        role="tabpanel"
        aria-labelledby={`${id}-tab-${filter}`}
      >
        {lines.length === 0 ? (
          <p className="border-b px-3.5 py-3 text-xs text-muted-foreground">
            Nothing in this group.
          </p>
        ) : (
          <ul>
            {lines.map((line) => {
              const label = statusLabel(line.status)
              return (
                <li key={line.key} className="flex gap-3 border-b px-3.5 py-3">
                  <span className="w-28 flex-none text-[13px]">
                    <StatusLabel tone={label.tone}>{label.text}</StatusLabel>
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col gap-[3px]">
                    <span className="text-[13px] font-medium">
                      {line.title}
                      {line.subtitle ? (
                        <span className="ml-1 text-xs font-normal text-muted-foreground">
                          {line.subtitle}
                        </span>
                      ) : null}
                    </span>
                    {line.reasons.map((reason, index) => (
                      <span
                        key={index}
                        className="text-xs text-muted-foreground"
                      >
                        {reason}
                      </span>
                    ))}
                    {line.paths ? (
                      <span className="font-mono text-[11px] break-words text-muted-foreground">
                        {line.paths}
                      </span>
                    ) : null}
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </div>
      {hasBlocks ? (
        <div className="border-b px-3.5 py-3">
          <button
            type="button"
            aria-expanded={showBlocks}
            aria-controls={`${id}-blocks`}
            className="rounded-sm text-xs text-muted-foreground underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => setShowBlocks((open) => !open)}
          >
            {showBlocks
              ? "Hide every option"
              : "Show every option, block by block"}
          </button>
          {showBlocks ? (
            <div id={`${id}-blocks`} className="mt-2">
              <BlockList report={report} />
            </div>
          ) : null}
        </div>
      ) : null}
      <p className="px-3.5 py-3 text-xs text-muted-foreground">{closingHint}</p>
      {waitsInHealth ? (
        <p className="flex flex-wrap items-baseline gap-x-2 border-t px-3.5 py-3 text-xs text-muted-foreground">
          Placeholders and unverified panels wait in Health.
          <Link
            to="/health"
            search={{ kind: importedFilter }}
            className="text-[13px] font-medium text-foreground hover:text-muted-foreground"
          >
            Open in Health
          </Link>
        </p>
      ) : null}
    </section>
  )
}
