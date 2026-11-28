import type { BlockWidth } from "./block-rows"
import type { Block } from "./queries"

export type Placement = {
  id: string
  width: BlockWidth
  height: number | null
}

export type Wanted<Body> = { layout: Placement[]; body: Body }

export type Queue<Body> = {
  inFlight: Wanted<Body> | null
  wanted: Wanted<Body> | null
}

export type Step<Body> = { queue: Queue<Body>; send: Wanted<Body> | null }

export function idle<Body>(): Queue<Body> {
  return { inFlight: null, wanted: null }
}

export function placements(blocks: readonly Block[]): Placement[] {
  return blocks
    .toSorted((a, b) => a.position - b.position)
    .map((block) => ({
      id: block.id,
      width: block.width,
      height: block.height ?? null,
    }))
}

function samePlacements(a: Placement[], b: Placement[]): boolean {
  return (
    a.length === b.length &&
    a.every(
      (entry, index) =>
        entry.id === b[index]?.id &&
        entry.width === b[index]?.width &&
        entry.height === b[index]?.height
    )
  )
}

export function want<Body>(
  queue: Queue<Body>,
  next: Wanted<Body>,
  current: Placement[]
): Step<Body> {
  if (queue.inFlight === null) {
    if (samePlacements(next.layout, current)) {
      return { queue, send: null }
    }
    return { queue: { inFlight: next, wanted: null }, send: next }
  }
  const pending = samePlacements(next.layout, queue.inFlight.layout)
    ? null
    : next
  return { queue: { inFlight: queue.inFlight, wanted: pending }, send: null }
}

export function settle<Body>(queue: Queue<Body>, written: boolean): Step<Body> {
  if (!written || queue.wanted === null) {
    return { queue: idle(), send: null }
  }
  return {
    queue: { inFlight: queue.wanted, wanted: null },
    send: queue.wanted,
  }
}
