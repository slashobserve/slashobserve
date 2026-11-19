import type uPlot from "uplot"

export type AnnotateRange = { fromMs: number; toMs?: number }

export type AnnotateGesture = uPlot.Plugin & {
  consumeSelect: () => boolean
}

const clickSlop = 4

export function annotateGesture(
  annotate: () => ((range: AnnotateRange) => void) | undefined
): AnnotateGesture {
  let armed = false
  return {
    consumeSelect: () => {
      const was = armed
      armed = false
      return was
    },
    hooks: {
      init: (u) => {
        u.over.addEventListener("mousedown", (down) => {
          armed = (down.metaKey || down.ctrlKey) && annotate() !== undefined
          if (!armed) return
          const release = (up: MouseEvent) => {
            document.removeEventListener("mouseup", release)
            const moved = Math.hypot(
              up.clientX - down.clientX,
              up.clientY - down.clientY
            )
            if (moved >= clickSlop) return
            armed = false
            const x = down.clientX - u.over.getBoundingClientRect().left
            annotate()?.({ fromMs: Math.round(u.posToVal(x, "x") * 1000) })
          }
          document.addEventListener("mouseup", release)
        })
      },
    },
  }
}
