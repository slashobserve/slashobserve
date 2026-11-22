import {
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react"
import { toast } from "sonner"

import {
  idle,
  placements,
  settle,
  want,
  type Queue,
  type Wanted,
} from "./arrange-queue"
import { blockWriteProblem } from "./errors"
import {
  applyDraft,
  arrangeBody,
  blockHeight,
  blockTitle,
  dropTarget,
  hasHeight,
  orderOf,
  placeBeside,
  singleColumnBelow,
  snapHeight,
  snapWidth,
  stepBlock,
  stepHeight,
  stepWidth,
  type BlockChange,
  type BlockRect,
  type BlockWidth,
  type Draft,
  type DropTarget,
} from "./block-rows"
import { useArrangeBlocks, type ArrangeBody, type Block } from "./queries"

const widthWords: Record<BlockWidth, string> = {
  third: "one third",
  half: "half",
  two_thirds: "two thirds",
  full: "full",
}

type Session =
  | { kind: "drag"; id: string }
  | {
      kind: "resize"
      id: string
      startX: number
      startY: number
      startWidth: number
      startHeight: number
      rowPixels: number
      base: Draft
    }

type Dragging = { id: string; drop: DropTarget | null }

export type Arrange = {
  blocks: Block[]
  arranging: boolean
  dragging: Dragging | null
  announcement: { message: string; count: number }
  startDrag: (block: Block) => (event: PointerEvent<HTMLElement>) => void
  moveDrag: (event: PointerEvent<HTMLElement>) => void
  endDrag: (event: PointerEvent<HTMLElement>) => void
  cancelDrag: () => void
  startResize: (block: Block) => (event: PointerEvent<HTMLElement>) => void
  moveResize: (event: PointerEvent<HTMLElement>) => void
  endResize: () => void
  cancelResize: () => void
  onKeyDown: (block: Block) => (event: KeyboardEvent<HTMLElement>) => void
  moveBy: (block: Block, direction: -1 | 1) => void
  setWidth: (block: Block, width: BlockWidth) => void
  resetHeight: (block: Block) => void
}

export function handleId(blockId: string): string {
  return `block-handle-${blockId}`
}

function readRects(list: HTMLElement): BlockRect[] {
  return Array.from(list.querySelectorAll<HTMLElement>("[data-block-id]")).map(
    (element) => {
      const rect = element.getBoundingClientRect()
      return {
        id: element.dataset.blockId ?? "",
        left: rect.left,
        top: rect.top,
        width: rect.width,
        height: rect.height,
      }
    }
  )
}

function holdPointer(element: HTMLElement, pointerId: number) {
  if (typeof element.setPointerCapture === "function") {
    element.setPointerCapture(pointerId)
  }
}

function focusHandle(blockId: string) {
  requestAnimationFrame(() =>
    document.getElementById(handleId(blockId))?.focus()
  )
}

export function useArrange({
  observationId,
  blocks,
  listRef,
}: {
  observationId: string
  blocks: readonly Block[]
  listRef: RefObject<HTMLElement | null>
}): Arrange {
  const mutation = useArrangeBlocks(observationId)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [dragging, setDragging] = useState<Dragging | null>(null)
  const [announcement, setAnnouncement] = useState({ message: "", count: 0 })
  const session = useRef<Session | null>(null)
  const queue = useRef<Queue<ArrangeBody>>(idle())

  const shown = useMemo(
    () => (draft === null ? [...blocks] : applyDraft(blocks, draft)),
    [blocks, draft]
  )

  function announce(message: string) {
    setAnnouncement((previous) => ({ message, count: previous.count + 1 }))
  }

  function baseDraft(): Draft {
    return draft ?? { order: orderOf(blocks), changes: {} }
  }

  function withChange(base: Draft, id: string, change: BlockChange): Draft {
    return {
      order: base.order,
      changes: { ...base.changes, [id]: { ...base.changes[id], ...change } },
    }
  }

  function send(item: Wanted<ArrangeBody>) {
    mutation.mutateAsync(item.body).then(
      () => advance(true),
      (error) => {
        toast.error(blockWriteProblem(error).text)
        advance(false)
      }
    )
  }

  function advance(written: boolean) {
    const step = settle(queue.current, written)
    queue.current = step.queue
    if (step.send === null) setDraft(null)
    else send(step.send)
  }

  function commit(next: Draft) {
    const step = want(
      queue.current,
      {
        layout: placements(applyDraft(blocks, next)),
        body: arrangeBody(next),
      },
      placements(blocks)
    )
    queue.current = step.queue
    if (step.queue.inFlight === null) setDraft(null)
    else setDraft(next)
    if (step.send !== null) send(step.send)
  }

  function startDrag(block: Block) {
    return (event: PointerEvent<HTMLElement>) => {
      if (event.button !== 0) return
      event.preventDefault()
      holdPointer(event.currentTarget, event.pointerId)
      session.current = { kind: "drag", id: block.id }
      setDragging({ id: block.id, drop: null })
    }
  }

  function moveDrag(event: PointerEvent<HTMLElement>) {
    const current = session.current
    const list = listRef.current
    if (current?.kind !== "drag" || list === null) return
    const drop = dropTarget(
      readRects(list),
      { x: event.clientX, y: event.clientY },
      current.id,
      list.clientWidth
    )
    setDragging({ id: current.id, drop })
  }

  function endDrag(event: PointerEvent<HTMLElement>) {
    const current = session.current
    const list = listRef.current
    session.current = null
    setDragging(null)
    if (current?.kind !== "drag" || list === null) return
    const drop = dropTarget(
      readRects(list),
      { x: event.clientX, y: event.clientY },
      current.id,
      list.clientWidth
    )
    if (drop === null) return
    const base = baseDraft()
    const order = placeBeside(
      orderOf(applyDraft(blocks, base)),
      current.id,
      drop
    )
    commit({ order, changes: base.changes })
    const place = order.indexOf(current.id) + 1
    announce(`${titleOf(current.id)}: position ${place} of ${order.length}`)
  }

  function cancelDrag() {
    session.current = null
    setDragging(null)
  }

  function startResize(block: Block) {
    return (event: PointerEvent<HTMLElement>) => {
      if (event.button !== 0) return
      event.preventDefault()
      event.stopPropagation()
      const list = listRef.current
      const section =
        event.currentTarget.closest<HTMLElement>("[data-block-id]")
      if (list === null || section === null) return
      holdPointer(event.currentTarget, event.pointerId)
      const current = shown.find((entry) => entry.id === block.id) ?? block
      session.current = {
        kind: "resize",
        id: block.id,
        startX: event.clientX,
        startY: event.clientY,
        startWidth: section.getBoundingClientRect().width,
        startHeight: blockHeight(current),
        rowPixels: list.clientWidth,
        base: baseDraft(),
      }
      setDraft(session.current.base)
    }
  }

  function resizedDraft(
    current: Extract<Session, { kind: "resize" }>,
    event: PointerEvent<HTMLElement>
  ): Draft {
    const block = blocks.find((entry) => entry.id === current.id)
    const change: BlockChange = {}
    if (current.rowPixels >= singleColumnBelow) {
      change.width = snapWidth(
        current.startWidth + event.clientX - current.startX,
        current.rowPixels
      )
    }
    if (block !== undefined && hasHeight(block)) {
      change.height = snapHeight(
        current.startHeight + event.clientY - current.startY
      )
    }
    return withChange(current.base, current.id, change)
  }

  function moveResize(event: PointerEvent<HTMLElement>) {
    const current = session.current
    if (current?.kind !== "resize") return
    setDraft(resizedDraft(current, event))
  }

  function endResize() {
    const current = session.current
    session.current = null
    if (current?.kind !== "resize") return
    const resized = draft ?? current.base
    commit(resized)
    const block = applyDraft(blocks, resized).find((b) => b.id === current.id)
    if (block !== undefined) {
      announce(`${blockTitle(block)}: ${describeSize(block)}`)
    }
  }

  function cancelResize() {
    session.current = null
    setDraft(null)
  }

  function titleOf(id: string): string {
    const block = blocks.find((entry) => entry.id === id)
    return block === undefined ? "Block" : blockTitle(block)
  }

  function moveBy(block: Block, direction: -1 | 1) {
    const base = baseDraft()
    const order = stepBlock(
      orderOf(applyDraft(blocks, base)),
      block.id,
      direction
    )
    if (
      order.indexOf(block.id) ===
      orderOf(applyDraft(blocks, base)).indexOf(block.id)
    ) {
      announce(`${blockTitle(block)}: can't move further`)
      return
    }
    commit({ order, changes: base.changes })
    announce(
      `${blockTitle(block)}: position ${order.indexOf(block.id) + 1} of ${order.length}`
    )
    focusHandle(block.id)
  }

  function arrangeSize(block: Block, change: BlockChange, unchanged: boolean) {
    if (unchanged) {
      announce(`${blockTitle(block)}: can't resize further`)
      return
    }
    const next = withChange(baseDraft(), block.id, change)
    commit(next)
    const resized = applyDraft(blocks, next).find((b) => b.id === block.id)
    if (resized !== undefined) {
      announce(`${blockTitle(resized)}: ${describeSize(resized)}`)
    }
    focusHandle(block.id)
  }

  function setWidth(block: Block, width: BlockWidth) {
    const current = shown.find((entry) => entry.id === block.id) ?? block
    arrangeSize(block, { width }, current.width === width)
  }

  function resetHeight(block: Block) {
    const current = shown.find((entry) => entry.id === block.id) ?? block
    arrangeSize(block, { height: null }, current.height === null)
  }

  function onKeyDown(block: Block) {
    return (event: KeyboardEvent<HTMLElement>) => {
      if (event.altKey || event.ctrlKey || event.metaKey) return
      const horizontal = event.key === "ArrowLeft" || event.key === "ArrowRight"
      const vertical = event.key === "ArrowUp" || event.key === "ArrowDown"
      if (!horizontal && !vertical) return
      event.preventDefault()
      const forward = event.key === "ArrowRight" || event.key === "ArrowDown"
      const direction = forward ? 1 : -1
      const current = shown.find((entry) => entry.id === block.id) ?? block
      if (!event.shiftKey) {
        moveBy(block, direction)
      } else if (horizontal) {
        const width = stepWidth(current.width, direction)
        arrangeSize(block, { width }, width === current.width)
      } else if (hasHeight(current)) {
        const height = stepHeight(blockHeight(current), direction)
        arrangeSize(block, { height }, height === blockHeight(current))
      } else {
        announce(`${blockTitle(block)}: has no height to change`)
      }
    }
  }

  return {
    blocks: shown,
    arranging: draft !== null || dragging !== null,
    dragging,
    announcement,
    startDrag,
    moveDrag,
    endDrag,
    cancelDrag,
    startResize,
    moveResize,
    endResize,
    cancelResize,
    onKeyDown,
    moveBy,
    setWidth,
    resetHeight,
  }
}

function describeSize(block: Block): string {
  const width = `${widthWords[block.width]} width`
  return hasHeight(block)
    ? `${width}, ${blockHeight(block)} pixels high`
    : width
}
