import type { LucideIcon } from "lucide-react"
import { useCallback, useLayoutEffect, useMemo, useRef } from "react"

import { rectWithin, surfaceOf, useAnchorPublisher } from "../anchor-layout"
import type { Size } from "../anchors"
import { scrollWithin, visibleBand } from "../list-anchors"
import { thresholdColor, thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import type { AlertAnchorLayout } from "./anchors"
import {
  alertModel,
  viewOf,
  type Alert,
  type AlertCounts,
  type AlertState,
} from "./alert-model"

const emptyText = "No firing or pending alerts."

const states: Record<
  AlertState,
  { word: string; Icon: LucideIcon; color: string }
> = {
  firing: {
    word: "Firing",
    Icon: thresholdIcons.critical,
    color: thresholdColor("critical"),
  },
  pending: {
    word: "Pending",
    Icon: thresholdIcons.warning,
    color: thresholdColor("warning"),
  },
}

function StateMark({ state }: { state: AlertState }) {
  const { word, Icon, color } = states[state]
  return (
    <span className="flex shrink-0 items-center gap-1.5">
      <Icon
        className="size-[13px]"
        strokeWidth={2.4}
        aria-hidden="true"
        style={{ color }}
      />
      {word}
    </span>
  )
}

function Message({ children, ok }: { children: string; ok?: boolean }) {
  const Icon = thresholdIcons.ok
  return (
    <p className="m-0 flex size-full items-center justify-center gap-1.5 p-3 text-center text-[13px] text-muted-foreground">
      {ok ? (
        <Icon
          className="size-[14px] shrink-0"
          strokeWidth={2.4}
          aria-hidden="true"
          style={{ color: thresholdColor("ok") }}
        />
      ) : null}
      {children}
    </p>
  )
}

function AlertRow({ alert }: { alert: Alert }) {
  return (
    <li
      data-state={alert.state}
      className="flex min-w-0 items-baseline gap-x-3 border-b px-3 py-1.5 text-[13px] last:border-b-0"
    >
      <span className="w-[72px] shrink-0 text-xs">
        <StateMark state={alert.state} />
      </span>
      <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
        <span className="font-medium">{alert.name}</span>
        {alert.severity ? (
          <span className="text-xs">{alert.severity}</span>
        ) : null}
        {alert.labels.map(([name, value]) => (
          <span key={name} className="text-xs text-muted-foreground">
            {name}={value}
          </span>
        ))}
      </span>
      {alert.activeFor ? (
        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
          {alert.activeFor}
        </span>
      ) : null}
    </li>
  )
}

function CountTiles({ counts }: { counts: AlertCounts }) {
  return (
    <div className="flex size-full">
      {(["firing", "pending"] as const).map((state) => (
        <div
          key={state}
          data-state={state}
          className="flex min-w-0 flex-1 flex-col justify-center gap-1 px-3"
        >
          <span className="text-[28px] leading-none font-medium tabular-nums">
            {counts[state]}
          </span>
          <span className="text-xs">
            <StateMark state={state} />
          </span>
        </div>
      ))}
    </div>
  )
}

function alertAnchorLayout(
  list: HTMLElement,
  alerts: readonly Alert[],
  body: Size,
  reveal: (key: string) => void
): AlertAnchorLayout {
  const root = surfaceOf(list)
  const view = rectWithin(list, root)
  const rows: AlertAnchorLayout["rows"][number][] = []
  const elsewhere: Alert[] = []
  alerts.forEach((alert, index) => {
    const element = list.children.item(index)
    const rect =
      element === null ? null : visibleBand(rectWithin(element, root), view)
    if (rect === null) elsewhere.push(alert)
    else rows.push({ rect, alert })
  })
  return { body, rows, elsewhere, reveal }
}

const AlertlistPanel: PanelRenderer = ({
  frames,
  display,
  meta,
  width,
  height,
}) => {
  const model = useMemo(
    () => alertModel(frames, display, Date.parse(meta.to)),
    [frames, display, meta.to]
  )
  const view = viewOf(display)
  const empty = model.total === 0
  const listed = !model.reason && !empty && view === "list"
  const listRef = useRef<HTMLUListElement>(null)
  const publish = useAnchorPublisher("alertlist")
  const reveal = useCallback(
    (key: string) => {
      const list = listRef.current
      const row = list?.children.item(
        model.alerts.findIndex((alert) => alert.key === key)
      )
      if (list && row) scrollWithin(list, row)
    },
    [model.alerts]
  )
  useLayoutEffect(() => {
    const list = listRef.current
    if (list === null) {
      publish(null)
      return
    }
    const measure = () =>
      publish(alertAnchorLayout(list, model.alerts, { width, height }, reveal))
    measure()
    list.addEventListener("scroll", measure, { passive: true })
    return () => list.removeEventListener("scroll", measure)
  }, [publish, listed, model.alerts, reveal, width, height])
  return (
    <div
      data-view={view}
      data-alerts={model.alerts.length}
      className="flex min-h-0 min-w-0 flex-col"
      style={{ width, height }}
    >
      {model.reason ? (
        <Message>{model.reason}</Message>
      ) : empty ? (
        <Message ok>{emptyText}</Message>
      ) : view === "stat" ? (
        <CountTiles counts={model.counts} />
      ) : (
        <>
          <ul
            ref={listRef}
            aria-label="Alerts"
            className="m-0 min-h-0 flex-1 list-none overflow-y-auto p-0"
          >
            {model.alerts.map((alert) => (
              <AlertRow key={alert.key} alert={alert} />
            ))}
          </ul>
          {model.total > model.alerts.length ? (
            <p className="m-0 shrink-0 border-t px-3 py-1 text-xs text-muted-foreground">
              Showing {model.alerts.length} of {model.total}.
            </p>
          ) : null}
        </>
      )}
    </div>
  )
}

export default AlertlistPanel
