import uPlot from "uplot"

import { resolveColor } from "@/panels/colors"

import { markerSpans } from "./markers"
import type { Annotation } from "./queries"

export type PlotPlace = {
  left: number
  top: number
  width: number
  height: number
  fromMs: number
  toMs: number
}

const bandOpacity = 0.1

export function annotationMarkersPlugin(
  annotations: () => readonly Annotation[],
  onPlace: (place: PlotPlace | null) => void
): uPlot.Plugin {
  return {
    hooks: {
      draw: (u) => {
        const { min, max } = u.scales.x
        if (min == null || max == null) return
        const fromMs = min * 1000
        const toMs = max * 1000
        const { left, top, width, height } = u.bbox
        const ratio = uPlot.pxRatio
        const spans = markerSpans(annotations(), { fromMs, toMs, width })
        if (spans.length > 0) {
          const { ctx } = u
          const color = resolveColor("var(--threshold-info)")
          const line = (x: number) => {
            const at = Math.round(left + x) + 0.5
            ctx.moveTo(at, top)
            ctx.lineTo(at, top + height)
          }
          ctx.save()
          ctx.beginPath()
          ctx.rect(left, top, width, height)
          ctx.clip()
          ctx.fillStyle = color
          ctx.globalAlpha = bandOpacity
          for (const { x, x2 } of spans) {
            if (x2 !== null) ctx.fillRect(left + x, top, x2 - x, height)
          }
          ctx.globalAlpha = 1
          ctx.strokeStyle = color
          ctx.lineWidth = Math.max(1, Math.round(ratio))
          ctx.beginPath()
          for (const { x, x2 } of spans) {
            line(x)
            if (x2 !== null) line(x2)
          }
          ctx.stroke()
          ctx.restore()
        }
        onPlace({
          left: left / ratio,
          top: top / ratio,
          width: width / ratio,
          height: height / ratio,
          fromMs,
          toMs,
        })
      },
      destroy: () => onPlace(null),
    },
  }
}
