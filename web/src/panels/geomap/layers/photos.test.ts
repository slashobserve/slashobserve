import { describe, expect, it } from "vitest"

import type { Tip } from "../geo-model"
import {
  maxPhotos,
  photosModel,
  truncatedPhotosText,
  type PhotoRow,
} from "./photos"

const tip: Tip = { label: "x", details: [], color: "c", state: null }
const origin = "https://app.test"

function row(index: number, url: unknown): PhotoRow {
  return { frameIndex: 0, row: index, lat: 1, lon: 2, url, tip }
}

describe("photosModel", () => {
  it("loads a remote https photo through the image proxy", () => {
    const { photos } = photosModel(
      [row(0, "https://img.example/a.png")],
      {},
      origin
    )
    expect(photos).toHaveLength(1)
    expect(photos[0].href).toBe(
      `/api/v1/assets/image?url=${encodeURIComponent("https://img.example/a.png")}`
    )
  })

  it("draws nothing for a URL that is not https or not text", () => {
    const { photos, truncated } = photosModel(
      [
        row(0, "http://img.example/a.png"),
        row(1, "javascript:alert(1)"),
        row(2, "data:image/png;base64,AAAA"),
        row(3, "//img.example/a.png"),
        row(4, ""),
        row(5, null),
        row(6, 42),
        row(7, "https://img.example/ok.png"),
      ],
      {},
      origin
    )
    expect(photos.map((photo) => photo.row)).toEqual([7])
    expect(truncated).toBe(0)
  })

  it("keeps at most 200 photos and counts the rest", () => {
    const rows = Array.from({ length: maxPhotos + 25 }, (_, index) =>
      row(index, `https://img.example/${index}.png`)
    )
    const model = photosModel(rows, {}, origin)
    expect(model.photos).toHaveLength(200)
    expect(model.truncated).toBe(25)
    expect(truncatedPhotosText(25)).toBe(
      "25 more photos were left off the map."
    )
  })

  it("does not count rejected photos toward the cap", () => {
    const rows = [
      ...Array.from({ length: 50 }, (_, index) =>
        row(index, "http://img.example/x.png")
      ),
      ...Array.from({ length: maxPhotos }, (_, index) =>
        row(50 + index, `https://img.example/${index}.png`)
      ),
    ]
    const model = photosModel(rows, {}, origin)
    expect(model.photos).toHaveLength(200)
    expect(model.truncated).toBe(0)
  })

  it("is square with a 2 pixel border unless the layer says otherwise", () => {
    expect(photosModel([], {}, origin)).toMatchObject({
      shape: "square",
      border: 2,
    })
    expect(
      photosModel([], { shape: "circle", border: 0 }, origin)
    ).toMatchObject({ shape: "circle", border: 0 })
  })
})
