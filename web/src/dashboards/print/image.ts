import { downloadFile } from "../export-spec"

const maxSide = 16384
const maxArea = 67_108_864
const wantedRatio = 2

export function imagePixelRatio(width: number, height: number): number {
  const bySide = maxSide / Math.max(width, height)
  const byArea = Math.sqrt(maxArea / (width * height))
  return Math.floor(Math.min(wantedRatio, bySide, byArea) * 100) / 100
}

export async function saveDashboardImage(
  element: HTMLElement,
  fileName: string
) {
  const { toBlob } = await import("html-to-image")
  const background = getComputedStyle(element)
    .getPropertyValue("--background")
    .trim()
  const blob = await toBlob(element, {
    pixelRatio: imagePixelRatio(element.scrollWidth, element.scrollHeight),
    ...(background === "" ? {} : { backgroundColor: background }),
  })
  if (blob === null) {
    throw new Error("The dashboard could not be drawn as an image")
  }
  downloadFile(blob, fileName)
}
