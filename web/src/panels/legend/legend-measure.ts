import { useEffect, useState } from "react"

import type { TextMeasure } from "./legend-width"

let measureContext: CanvasRenderingContext2D | null = null
const averageGlyphEm = 0.6

function legendTextMeasure(rem: number): TextMeasure {
  measureContext ??= document.createElement("canvas").getContext("2d")
  const context = measureContext
  const size = 0.75 * rem
  if (context === null) return (text) => text.length * averageGlyphEm * size
  const family = getComputedStyle(document.documentElement).fontFamily
  return (text, weight) => {
    context.font = `${weight} ${size}px ${family}`
    return context.measureText(text).width
  }
}

const newMeasure = (): typeof legendTextMeasure => (rem) =>
  legendTextMeasure(rem)

export function useLegendTextMeasure(): typeof legendTextMeasure {
  const [measure, setMeasure] = useState(newMeasure)
  useEffect(() => {
    if (document.fonts.status === "loaded") return
    let current = true
    void document.fonts.ready.then(() => {
      if (current) setMeasure(newMeasure)
    })
    return () => {
      current = false
    }
  }, [])
  return measure
}
