import { rectWithin, surfaceOf } from "./anchor-layout"
import type { Rect, Size } from "./anchors"
import type { ValueAnchorLayout } from "./value-anchors"

type DrawnValue = {
  key: string
  name: string
  labels: Record<string, string> | undefined
  text: string
}

function shownWithin(rect: Rect, view: Rect): Rect | null {
  const left = Math.max(rect.x, view.x)
  const top = Math.max(rect.y, view.y)
  const right = Math.min(rect.x + rect.width, view.x + view.width)
  const bottom = Math.min(rect.y + rect.height, view.y + view.height)
  const middleX = rect.x + rect.width / 2
  const middleY = rect.y + rect.height / 2
  const middleShown =
    middleX >= view.x &&
    middleX <= view.x + view.width &&
    middleY >= view.y &&
    middleY <= view.y + view.height
  if (!middleShown || right < left || bottom < top) return null
  return { x: left, y: top, width: right - left, height: bottom - top }
}

export function valueAnchorLayout(
  container: HTMLElement,
  values: readonly DrawnValue[],
  body: Size
): ValueAnchorLayout {
  const root = surfaceOf(container)
  const view = rectWithin(container, root)
  const drawn = new Map(
    Array.from(
      container.querySelectorAll<HTMLElement>("[data-value-key]"),
      (element) => [element.dataset.valueKey, element] as const
    )
  )
  return {
    body,
    targets: values.flatMap(({ key, name, labels, text }) => {
      const element = drawn.get(key)
      if (element === undefined) return []
      const rect = shownWithin(rectWithin(element, root), view)
      return [{ rect, name, labels, text }]
    }),
  }
}
