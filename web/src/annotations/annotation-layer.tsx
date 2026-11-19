import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type PointerEvent,
} from "react"
import { createPortal } from "react-dom"
import { ArrowUpRightIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { zonedDateTime } from "@/lib/time"

import { useAnnotationActions } from "./actions"
import {
  cardLeft,
  clusterFlags,
  linkParts,
  markerSpans,
  type MarkerFlag,
} from "./markers"
import type { Annotation } from "./queries"
import { timeWithZone } from "./time"

const flagWidth = 10
const flagHeight = 8
const viewportMargin = 8
const cardWidth = 280
const labelTextLength = 80
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
          height={height}
          zone={zone}
        />
      ))}
    </div>
  )
}

function timeText(annotation: Annotation, zone: string | undefined): string {
  const from = timeWithZone(annotation.time_from, zone)
  return annotation.time_to === null
    ? from
    : `${from} to ${timeWithZone(annotation.time_to, zone)}`
}

function flagLabel(flag: MarkerFlag, zone: string | undefined): string {
  const newest = flag.annotations[0]
  if (newest === undefined) return "Annotation"
  const text = newest.text.slice(0, labelTextLength)
  const more = flag.annotations.length - 1
  const label = `Annotation: ${text}, ${zonedDateTime(newest.time_from, zone)}`
  return more > 0 ? `${label} and ${more} more` : label
}

type Anchor = { x: number; top: number }

function Flag({
  flag,
  height,
  zone,
}: {
  flag: MarkerFlag
  height: number
  zone: string | undefined
}) {
  const [anchor, setAnchor] = useState<Anchor | null>(null)
  const open = anchor !== null
  const flagRef = useRef<HTMLButtonElement>(null)
  const setOpen = (next: boolean | ((current: boolean) => boolean)) => {
    const wanted = typeof next === "function" ? next(open) : next
    if (!wanted) {
      setAnchor(null)
      return
    }
    const rect = flagRef.current?.getBoundingClientRect()
    if (rect) setAnchor({ x: rect.left + rect.width / 2, top: rect.top })
  }

  useEffect(() => {
    if (!open) return
    const close = () => setAnchor(null)
    window.addEventListener("scroll", close, true)
    window.addEventListener("resize", close)
    return () => {
      window.removeEventListener("scroll", close, true)
      window.removeEventListener("resize", close)
    }
  }, [open])

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
      {anchor
        ? createPortal(
            <Card
              annotations={flag.annotations}
              anchor={anchor}
              zone={zone}
              onAct={() => setOpen(false)}
            />,
            document.body
          )
        : null}
    </div>
  )
}

function Card({
  annotations,
  anchor,
  zone,
  onAct,
}: {
  annotations: readonly Annotation[]
  anchor: Anchor
  zone: string | undefined
  onAct: () => void
}) {
  return (
    <div
      role="group"
      aria-label="Annotation"
      className="pointer-events-auto fixed z-50 flex flex-col gap-2 overflow-auto rounded-card border border-border bg-popover p-2 text-xs text-popover-foreground"
      style={{
        left:
          cardLeft(
            anchor.x - viewportMargin,
            cardWidth,
            window.innerWidth - 2 * viewportMargin
          ) + viewportMargin,
        bottom: window.innerHeight - anchor.top,
        width: cardWidth,
        maxHeight: Math.max(0, anchor.top - viewportMargin),
      }}
    >
      {annotations.map((annotation) => (
        <Entry
          key={annotation.id}
          annotation={annotation}
          zone={zone}
          onAct={onAct}
        />
      ))}
    </div>
  )
}

function Entry({
  annotation,
  zone,
  onAct,
}: {
  annotation: Annotation
  zone: string | undefined
  onAct: () => void
}) {
  const actions = useAnnotationActions()
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
      {actions?.canChange(annotation) ? (
        <div className="flex gap-1">
          <Button
            variant="ghost"
            size="xs"
            onClick={() => {
              onAct()
              actions.edit(annotation)
            }}
          >
            Edit
          </Button>
          <Button
            variant="ghost"
            size="xs"
            onClick={() => {
              onAct()
              actions.remove(annotation)
            }}
          >
            Delete
          </Button>
        </div>
      ) : null}
    </div>
  )
}
