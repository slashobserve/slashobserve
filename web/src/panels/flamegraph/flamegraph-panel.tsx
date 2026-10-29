import { useMemo, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useTheme } from "@/lib/theme"

import { displayValue } from "../field-config"
import type { PanelRenderer } from "../types"
import { FlameCanvas, type Hover } from "./flame-canvas"
import {
  diffNote,
  flameModel,
  type FlameModel,
  focusText,
  fullRange,
  search,
  shareText,
  zoomForFocus,
  zoomTo,
  type FlameProfile,
} from "./flame-model"

const padX = 12
const padY = 6
const toolbarHeight = 32
const toolbarGap = 6
const tooltipOffset = 12

function Message({
  text,
  width,
  height,
}: {
  text: string
  width: number
  height: number
}) {
  return (
    <div
      className="flex items-center justify-center p-4 text-center text-xs text-muted-foreground"
      style={{ width, height }}
    >
      {text}
    </div>
  )
}

function Tooltip({
  profile,
  hover,
  bounds,
}: {
  profile: FlameProfile
  hover: Hover
  bounds: DOMRect
}) {
  const node = profile.nodes[hover.index]
  const x = hover.clientX - bounds.left
  const y = hover.clientY - bounds.top
  const onRight = x > bounds.width / 2
  return (
    <div
      role="tooltip"
      className="pointer-events-none absolute z-10 max-w-72 min-w-40 rounded-control border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground"
      style={{
        top: Math.min(y + tooltipOffset, Math.max(0, bounds.height - 72)),
        ...(onRight
          ? { right: bounds.width - x + tooltipOffset }
          : { left: x + tooltipOffset }),
      }}
    >
      <div className="mb-1 font-medium break-words">{node.label}</div>
      <dl className="m-0 grid grid-cols-[auto_auto] justify-between gap-x-3 tabular-nums">
        <dt className="text-muted-foreground">Total</dt>
        <dd className="m-0">
          {displayValue(node.value, profile.config).text} (
          {shareText(node.share)})
        </dd>
        <dt className="text-muted-foreground">Self</dt>
        <dd className="m-0">{displayValue(node.self, profile.config).text}</dd>
      </dl>
    </div>
  )
}

const FlameGraphPanel: PanelRenderer = ({ frames, fields, width, height }) => {
  const { resolved: theme } = useTheme()
  const [hover, setHover] = useState<Hover | null>(null)
  const [query, setQuery] = useState("")
  const [zoom, setZoom] = useState<{ model: FlameModel; index: number } | null>(
    null
  )
  const [focus, setFocus] = useState<{
    model: FlameModel
    index: number | null
  } | null>(null)
  const [root, setRoot] = useState<HTMLDivElement | null>(null)
  const model = useMemo(() => flameModel(frames, fields), [frames, fields])

  if (model.reason !== null) {
    return <Message text={model.reason} width={width} height={height} />
  }
  if (model.nodes.length === 0 || model.total <= 0) {
    return <Message text="No profile to show." width={width} height={height} />
  }

  const zoomed = zoom !== null && zoom.model === model ? zoom.index : null
  const focused = focus !== null && focus.model === model ? focus.index : null
  const range = zoomed === null ? fullRange(model) : zoomTo(model, zoomed)
  const matches = search(model, query)
  const plotWidth = width - 2 * padX
  const bodyHeight = height - padY - toolbarHeight - toolbarGap
  const bounds = hover && root ? root.getBoundingClientRect() : null

  return (
    <div
      ref={setRoot}
      data-nodes={model.nodes.length}
      className="relative flex flex-col"
      style={{ width, height, padding: `${padY}px ${padX}px 0` }}
    >
      <div
        className="flex items-center gap-2"
        style={{ height: toolbarHeight, marginBottom: toolbarGap }}
      >
        <Input
          type="search"
          aria-label="Search frames"
          placeholder="Search frames"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="h-7 max-w-64"
        />
        <Button
          variant="outline"
          size="sm"
          disabled={zoomed === null}
          onClick={() => setZoom(null)}
        >
          Reset
        </Button>
        {model.diff ? (
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {diffNote}
          </span>
        ) : null}
      </div>
      {plotWidth > 0 && bodyHeight > 0 ? (
        <div
          className="overflow-x-hidden overflow-y-auto"
          style={{ width: plotWidth, height: bodyHeight }}
        >
          <FlameCanvas
            profile={model}
            range={range}
            matches={matches}
            width={plotWidth}
            theme={theme}
            focused={focused}
            label={`Flame graph with ${model.nodes.length} frames and ${model.levels.length} levels`}
            onHover={setHover}
            onSelect={(index) => {
              setHover(null)
              setZoom({ model, index })
            }}
            onFocusChange={(index) => {
              setFocus({ model, index })
              if (index === null) return
              const next = zoomForFocus(model, zoomed, index)
              if (next !== zoomed) {
                setZoom(next === null ? null : { model, index: next })
              }
            }}
            onReset={zoomed === null ? null : () => setZoom(null)}
          />
        </div>
      ) : null}
      <div aria-live="polite" className="sr-only">
        {focused === null ? "" : focusText(model, focused)}
      </div>
      {hover && bounds && hover.index < model.nodes.length ? (
        <Tooltip profile={model} hover={hover} bounds={bounds} />
      ) : null}
    </div>
  )
}

export default FlameGraphPanel
