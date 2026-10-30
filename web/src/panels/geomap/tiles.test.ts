import { describe, expect, it } from "vitest"

import { maxTiles, visibleTiles } from "./tiles"

const centered = (zoom: number) => ({ center: { x: 0.5, y: 0.5 }, zoom })

describe("visibleTiles", () => {
  it("draws the one world tile at zoom 0 in a 256px body", () => {
    const tiles = visibleTiles(centered(0), 256, 256)
    expect(tiles).toHaveLength(1)
    expect(tiles[0]).toMatchObject({ z: 0, x: 0, y: 0, size: 256 })
    expect(Math.abs(tiles[0].left)).toBe(0)
    expect(Math.abs(tiles[0].top)).toBe(0)
  })

  it("scales tiles between whole zoom levels", () => {
    const tiles = visibleTiles(centered(2.5), 400, 300)
    expect(tiles.every((tile) => tile.z === 2)).toBe(true)
    for (const tile of tiles) expect(tile.size).toBeCloseTo(256 * Math.SQRT2, 8)
  })

  it("covers the whole body", () => {
    const tiles = visibleTiles(
      { center: { x: 0.3, y: 0.6 }, zoom: 4.2 },
      500,
      320
    )
    const left = Math.min(...tiles.map((tile) => tile.left))
    const top = Math.min(...tiles.map((tile) => tile.top))
    const right = Math.max(...tiles.map((tile) => tile.left + tile.size))
    const bottom = Math.max(...tiles.map((tile) => tile.top + tile.size))
    expect(left).toBeLessThanOrEqual(0)
    expect(top).toBeLessThanOrEqual(0)
    expect(right).toBeGreaterThanOrEqual(500)
    expect(bottom).toBeGreaterThanOrEqual(320)
  })

  it("wraps columns across the antimeridian", () => {
    const tiles = visibleTiles(
      { center: { x: 0.99, y: 0.5 }, zoom: 3 },
      600,
      256
    )
    const columns = new Set(tiles.map((tile) => tile.x))
    expect(columns.has(7)).toBe(true)
    expect(columns.has(0)).toBe(true)
    expect(tiles.every((tile) => tile.x >= 0 && tile.x < 8)).toBe(true)
  })

  it("skips rows above and below the world", () => {
    const tiles = visibleTiles(centered(0), 256, 800)
    expect(tiles.map((tile) => tile.y)).toEqual([0])
    expect(tiles.every((tile) => tile.y >= 0 && tile.y < 1)).toBe(true)
  })

  it("keeps a huge body within the tile budget by using a coarser level", () => {
    const view = { center: { x: 0.5, y: 0.5 }, zoom: 8 }
    const tiles = visibleTiles(view, 4000, 4000)
    expect(tiles.length).toBeLessThanOrEqual(maxTiles)
    expect(tiles[0].z).toBeLessThan(8)
  })

  it("never asks for a level above the given maximum", () => {
    const tiles = visibleTiles(centered(12), 300, 300, 6)
    expect(tiles.every((tile) => tile.z <= 6)).toBe(true)
    expect(tiles[0].size).toBeCloseTo(256 * 2 ** 6, 6)
  })
})
