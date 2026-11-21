import { rectWithin, surfaceOf } from "./anchor-layout"
import type { Size } from "./anchors"
import type { ValueAnchorLayout } from "./value-anchors"

type DrawnValue = {
  name: string
  labels: Record<string, string> | undefined
  text: string
}

export function valueAnchorLayout(
  container: HTMLElement,
  values: readonly DrawnValue[],
  body: Size
): ValueAnchorLayout {
  const root = surfaceOf(container)
  return {
    body,
    targets: values.flatMap(({ name, labels, text }, index) => {
      const element = container.children.item(index)
      if (element === null) return []
      return [{ rect: rectWithin(element, root), name, labels, text }]
    }),
  }
}
