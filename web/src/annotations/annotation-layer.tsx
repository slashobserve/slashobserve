import {
  useMemo,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type PointerEvent,
} from "react"
import { ArrowUpRightIcon } from "lucide-react"

import { zonedDateTime } from "@/lib/time"

import {
  cardLeft,
  clusterFlags,
  linkParts,
  markerSpans,
  type MarkerFlag,
} from "./markers"
import type { Annotation } from "./queries"

const flagWidth = 10
const flagHeight = 8
const cardWidth = 280
const labelTextLength = 80
const maxCardEntries = 5
const textLinkClass = "font-medium hover:text-muted-foreground"

type LayerProps = {
  annotations: readonly Annotation[]
  fromMs: number
  toMs: number
  left: number
  top: number
  width: number
  height: number
  zone: string | undefined
}

export function AnnotationLayer({
  annotations,
  fromMs,
  toMs,
  left,
  top,
  width,
  height,
  zone,
}: LayerProps) {
  const flags = useMemo(
    () => clusterFlags(markerSpans(annotations, { fromMs, toMs, width })),
    [annotations, fromMs, toMs, width]
  )
  if (flags.length === 0) return null
  return (
    <div
      className="pointer-events-none absolute"
      style={{ left, top, width, height }}
    >
      {flags.map((flag) => (
        <Flag
          key={`${flag.annotations[0]?.id ?? ""}:${flag.x}`}
          flag={flag}
          width={width}
          height={height}
          spaceAbove={height - flagHeight + top}
          zone={zone}
        />
      ))}
    </div>
  )
}

function timeText(annotation: Annotation, zone: string | undefined): string {
  const from = zonedDateTime(annotation.time_from, zone)
  return annotation.time_to === null
    ? from
    : `${from} to ${zonedDateTime(annotation.time_to, zone)}`
}

function flagLabel(flag: MarkerFlag, zone: string | undefined): string {
  const newest = flag.annotations[0]
  if (newest === undefined) return "Annotation"
  const text = newest.text.slice(0, labelTextLength)
  const more = flag.annotations.length - 1
  const label = `Annotation: ${text}, ${zonedDateTime(newest.time_from, zone)}`
  return more > 0 ? `${label} and ${more} more` : label
}

function Flag({
  flag,
  width,
  height,
  spaceAbove,
  zone,
}: {
  flag: MarkerFlag
  width: number
  height: number
  spaceAbove: number
  zone: string | undefined
}) {
  const [open, setOpen] = useState(false)
  const flagRef = useRef<HTMLButtonElement>(null)

  const enter = (event: PointerEvent) => {
    if (event.pointerType !== "touch") setOpen(true)
  }
  const leave = (event: PointerEvent) => {
    if (event.pointerType !== "touch") setOpen(false)
  }
  const blur = (event: FocusEvent<HTMLDivElement>) => {
    if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false)
  }
  const keyDown = (event: KeyboardEvent) => {
    if (event.key !== "Escape" || !open) return
    event.stopPropagation()
    setOpen(false)
    flagRef.current?.focus()
  }

  return (
    <div
      className="absolute top-0 left-0"
      onPointerEnter={enter}
      onPointerLeave={leave}
      onBlur={blur}
      onKeyDown={keyDown}
    >
      <button
        ref={flagRef}
        type="button"
        aria-label={flagLabel(flag, zone)}
        aria-expanded={open}
        className="pointer-events-auto absolute block rounded-none bg-threshold-info p-0 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        style={{
          left: flag.x - flagWidth / 2,
          top: height - flagHeight,
          width: flagWidth,
          height: flagHeight,
        }}
        onMouseDown={(event) => event.stopPropagation()}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen((value) => !value)}
      />
      {open ? (
        <Card
          annotations={flag.annotations}
          left={cardLeft(flag.x, cardWidth, width)}
          bottom={height - flagHeight}
          maxHeight={Math.max(0, spaceAbove)}
          zone={zone}
        />
      ) : null}
    </div>
  )
}

function Card({
  annotations,
  left,
  bottom,
  maxHeight,
  zone,
}: {
  annotations: readonly Annotation[]
  left: number
  bottom: number
  maxHeight: number
  zone: string | undefined
}) {
  const shown = annotations.slice(0, maxCardEntries)
  const more = annotations.length - shown.length
  return (
    <div
      role="group"
      aria-label="Annotation"
      className="pointer-events-auto absolute z-20 flex -translate-y-full flex-col gap-2 overflow-auto rounded-card border border-border bg-popover p-2 text-xs text-popover-foreground"
      style={{ left, top: bottom, width: cardWidth, maxHeight }}
    >
      {shown.map((annotation) => (
        <Entry key={annotation.id} annotation={annotation} zone={zone} />
      ))}
      {more > 0 ? (
        <div className="text-muted-foreground">{more} more</div>
      ) : null}
    </div>
  )
}

function Entry({
  annotation,
  zone,
}: {
  annotation: Annotation
  zone: string | undefined
}) {
  const parts = linkParts(annotation.text, window.location.origin)
  return (
    <div className="flex flex-col gap-1">
      <div className="text-muted-foreground tabular-nums">
        {timeText(annotation, zone)}
      </div>
      <div className="text-muted-foreground">
        {annotation.actor?.label ?? "Unknown"}
        {annotation.dashboard_id === null ? " · Every dashboard" : null}
      </div>
      <p className="text-[13px] break-words whitespace-pre-wrap">
        {parts.map((part, i) =>
          part.href === undefined ? (
            <span key={i}>{part.text}</span>
          ) : part.href.startsWith("/") ? (
            <a key={i} href={part.href} className={textLinkClass}>
              {part.text}
            </a>
          ) : (
            <a
              key={i}
              href={part.href}
              target="_blank"
              rel="noopener noreferrer"
              className={textLinkClass}
            >
              {part.text}
              <ArrowUpRightIcon
                aria-hidden="true"
                className="ml-0.5 inline size-3"
              />
            </a>
          )
        )}
      </p>
      {annotation.tags.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {annotation.tags.map((tag) => (
            <span
              key={tag}
              className="rounded-sm border border-border px-1 text-muted-foreground"
            >
              {tag}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  )
}
