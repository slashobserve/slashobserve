import { useCallback, useLayoutEffect, useRef, type RefObject } from "react"

import type { CursorMember, SharedCursor } from "./shared-cursor"

type LineRef = RefObject<HTMLDivElement | null>

function paint(
  line: HTMLDivElement | null,
  peer: number | null,
  toLeft: (x: number) => number | null
) {
  if (!line) return
  const left = peer === null ? null : toLeft(peer)
  if (left === null) {
    line.style.visibility = "hidden"
    return
  }
  line.style.transform = `translateX(${left}px)`
  line.style.visibility = "visible"
}

export function useCursorLine(
  cursor: SharedCursor,
  toLeft: (x: number) => number | null
): { lineRef: LineRef; move: (x: number | null) => void } {
  const lineRef = useRef<HTMLDivElement>(null)
  const member = useRef<CursorMember | null>(null)
  const place = useRef(toLeft)
  const peer = useRef<number | null>(null)

  useLayoutEffect(() => {
    const joined = cursor.join((next) => {
      peer.current = next === null ? null : next.x
      paint(lineRef.current, peer.current, place.current)
    })
    member.current = joined
    return () => {
      joined.leave()
      member.current = null
    }
  }, [cursor])

  useLayoutEffect(() => {
    place.current = toLeft
    paint(lineRef.current, peer.current, toLeft)
  })

  const move = useCallback((x: number | null) => {
    if (x !== null) {
      peer.current = null
      paint(lineRef.current, null, place.current)
    }
    member.current?.move(x)
  }, [])

  return { lineRef, move }
}

export function CursorLine({
  lineRef,
  height,
}: {
  lineRef: LineRef
  height: number
}) {
  return (
    <div
      ref={lineRef}
      aria-hidden="true"
      className="pointer-events-none absolute top-0 left-0 border-l border-dashed border-chart-axis"
      style={{ height, visibility: "hidden" }}
    />
  )
}
