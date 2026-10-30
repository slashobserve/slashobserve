export type Rgb = readonly [number, number, number]

export const lightText = "#ffffff"
export const darkText = "#0a0a0a"

const lightRgb: Rgb = [255, 255, 255]
const darkRgb: Rgb = [10, 10, 10]

function linear(channel: number): number {
  const unit = channel / 255
  return unit <= 0.03928 ? unit / 12.92 : ((unit + 0.055) / 1.055) ** 2.4
}

function luminance([red, green, blue]: Rgb): number {
  return 0.2126 * linear(red) + 0.7152 * linear(green) + 0.0722 * linear(blue)
}

export function contrastRatio(a: Rgb, b: Rgb): number {
  const [lighter, darker] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (lighter + 0.05) / (darker + 0.05)
}

export function readableText(
  background: Rgb
): typeof lightText | typeof darkText {
  return contrastRatio(background, lightRgb) >=
    contrastRatio(background, darkRgb)
    ? lightText
    : darkText
}

const hex = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})(?:[0-9a-f]{2})?$/i
const functional = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)/i

export function parseRgb(normalized: string): Rgb | null {
  const hexMatch = hex.exec(normalized.trim())
  if (hexMatch) {
    return [
      parseInt(hexMatch[1], 16),
      parseInt(hexMatch[2], 16),
      parseInt(hexMatch[3], 16),
    ]
  }
  const match = functional.exec(normalized.trim())
  return match ? [Number(match[1]), Number(match[2]), Number(match[3])] : null
}

let swatch: CanvasRenderingContext2D | null | undefined

function pixelRgb(context: CanvasRenderingContext2D): Rgb | null {
  context.clearRect(0, 0, 1, 1)
  context.fillRect(0, 0, 1, 1)
  const [red, green, blue, alpha] = context.getImageData(0, 0, 1, 1).data
  return alpha === 0 ? null : [red, green, blue]
}

export function cssColorToRgb(color: string): Rgb | null {
  if (!CSS.supports("color", color)) return null
  swatch ??= document
    .createElement("canvas")
    .getContext("2d", { willReadFrequently: true })
  const context = swatch
  if (!context) return null
  context.fillStyle = color
  return parseRgb(context.fillStyle) ?? pixelRgb(context)
}
