import { Link } from "@tanstack/react-router"
import { MinusIcon, PlusIcon, RotateCcwIcon, RulerIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

import { formatImperial, formatMetric, scaleBars } from "./scale"

const scaleBarMaxPixels = 100

export function ZoomControls({
  canZoomIn,
  canZoomOut,
  canReset,
  onZoomIn,
  onZoomOut,
  onReset,
}: {
  canZoomIn: boolean
  canZoomOut: boolean
  canReset: boolean
  onZoomIn: () => void
  onZoomOut: () => void
  onReset: () => void
}) {
  return (
    <>
      <Button
        variant="outline"
        size="icon-sm"
        aria-label="Zoom in"
        disabled={!canZoomIn}
        onClick={onZoomIn}
      >
        <PlusIcon />
      </Button>
      <Button
        variant="outline"
        size="icon-sm"
        aria-label="Zoom out"
        disabled={!canZoomOut}
        onClick={onZoomOut}
      >
        <MinusIcon />
      </Button>
      <Button
        variant="outline"
        size="icon-sm"
        aria-label="Reset view"
        disabled={!canReset}
        onClick={onReset}
      >
        <RotateCcwIcon />
      </Button>
    </>
  )
}

export function MeasureToggle({
  active,
  onToggle,
}: {
  active: boolean
  onToggle: () => void
}) {
  return (
    <Button
      variant={active ? "secondary" : "outline"}
      size="icon-sm"
      aria-label="Measure"
      aria-pressed={active}
      onClick={onToggle}
    >
      <RulerIcon />
    </Button>
  )
}

export function MeasureReadout({
  points,
  meters,
}: {
  points: number
  meters: number
}) {
  return (
    <div
      role="status"
      className="pointer-events-none absolute top-2 left-11 rounded-control border border-border bg-background px-2 py-1 text-xs tabular-nums"
    >
      {points < 2
        ? "Click the map to add points. Enter adds the centre, Escape clears."
        : `Distance ${formatMetric(meters)} (${formatImperial(meters)})`}
    </div>
  )
}

function Bar({ label, pixels }: { label: string; pixels: number }) {
  return (
    <div className="flex flex-col" style={{ width: pixels }}>
      <span className="truncate leading-none">{label}</span>
      <span className="h-1.5 border-x border-b border-foreground" />
    </div>
  )
}

export function ScaleBars({
  latitude,
  zoom,
}: {
  latitude: number
  zoom: number
}) {
  const { metric, imperial } = scaleBars(latitude, zoom, scaleBarMaxPixels)
  return (
    <div
      aria-label="Map scale"
      className="pointer-events-none absolute bottom-6 left-2 flex flex-col gap-1 text-xs text-foreground [text-shadow:0_0_3px_var(--background)]"
    >
      <Bar {...metric} />
      <Bar {...imperial} />
    </div>
  )
}

export function DebugLine({
  zoom,
  latitude,
  longitude,
}: {
  zoom: number
  latitude: number
  longitude: number
}) {
  return (
    <span className="shrink-0 tabular-nums">
      Zoom {zoom.toFixed(2)}, centre {latitude.toFixed(3)},{" "}
      {longitude.toFixed(3)}
    </span>
  )
}

export function BlockedTilesNote({ host }: { host: string }) {
  return (
    <span className="pointer-events-auto">
      Map tiles from {host} are not allowed on this server.{" "}
      <Link
        to="/settings"
        hash="outbound-access"
        className="rounded-sm font-medium text-foreground underline underline-offset-2 outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        Outbound access
      </Link>
    </span>
  )
}
