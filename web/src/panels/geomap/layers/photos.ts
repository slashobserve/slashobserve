import { assetImageUrl } from "@/lib/remote-assets"

import type { Tip } from "../geo-model"
import { project } from "../projection"

export type PhotoRow = {
  frameIndex: number
  row: number
  lat: number
  lon: number
  url: unknown
  tip: Tip
}

type Photo = {
  key: string
  frameIndex: number
  row: number
  lat: number
  lon: number
  x: number
  y: number
  href: string
  tip: Tip
}

export type PhotosModel = {
  photos: Photo[]
  truncated: number
  shape: "square" | "circle"
  border: number
}

export const maxPhotos = 200
export const photoSize = 36

const defaultBorder = 2

function hrefOf(url: unknown, origin: string | undefined): string | null {
  if (typeof url !== "string") return null
  const text = url.trim()
  if (!text.startsWith("https://")) return null
  return assetImageUrl(text, origin)
}

export function truncatedPhotosText(count: number): string {
  return `${count} more photos were left off the map.`
}

export function photosModel(
  rows: readonly PhotoRow[],
  layer: Record<string, unknown>,
  origin?: string
): PhotosModel {
  const photos: Photo[] = []
  let truncated = 0
  for (const row of rows) {
    const href = hrefOf(row.url, origin)
    if (href === null) continue
    if (photos.length >= maxPhotos) {
      truncated++
      continue
    }
    photos.push({
      key: `${row.frameIndex}:${row.row}`,
      frameIndex: row.frameIndex,
      row: row.row,
      lat: row.lat,
      lon: row.lon,
      href,
      tip: row.tip,
      ...project(row.lon, row.lat),
    })
  }
  return {
    photos,
    truncated,
    shape: layer.shape === "circle" ? "circle" : "square",
    border:
      typeof layer.border === "number" && layer.border >= 0
        ? layer.border
        : defaultBorder,
  }
}
