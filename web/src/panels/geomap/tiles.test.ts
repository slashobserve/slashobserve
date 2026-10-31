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
    const tiles = visibleTiles(centered(12), 300, 300, { maxLevel: 6 })
    expect(tiles.every((tile) => tile.z <= 6)).toBe(true)
    expect(tiles[0].size).toBeCloseTo(256 * 2 ** 6, 6)
  })

  it("never asks for a level below the given minimum", () => {
    const tiles = visibleTiles(centered(1.5), 300, 300, { minLevel: 3 })
    expect(tiles.length).toBeGreaterThan(0)
    expect(tiles.every((tile) => tile.z === 3)).toBe(true)
    expect(tiles[0].size).toBeCloseTo(256 * 2 ** -1.5, 6)
  })

  it("keeps the minimum level when a coarser one would cut the tile budget", () => {
    const view = { center: { x: 0.5, y: 0.5 }, zoom: 8 }
    const tiles = visibleTiles(view, 1200, 1200, { minLevel: 6 })
    expect(tiles.length).toBeLessThanOrEqual(maxTiles)
    expect(tiles.every((tile) => tile.z >= 6)).toBe(true)
  })

  it("draws no tiles when the minimum level alone exceeds the tile budget", () => {
    const view = { center: { x: 0.5, y: 0.5 }, zoom: 8 }
    expect(visibleTiles(view, 4000, 4000, { minLevel: 8 })).toEqual([])
  })

  it("draws the world once with repeat off", () => {
    const view = { center: { x: 0.5, y: 0.5 }, zoom: 1 }
    const repeated = visibleTiles(view, 1500, 512)
    const once = visibleTiles(view, 1500, 512, { repeat: false })
    expect(repeated.some((tile) => tile.column < 0 || tile.column > 1)).toBe(
      true
    )
    expect(once.map((tile) => tile.column).sort()).toEqual([0, 0, 1, 1])
    expect(once.every((tile) => tile.column >= 0 && tile.column < 2)).toBe(true)
  })

  it("draws nothing sideways of the world when the view is past its edge", () => {
    const view = { center: { x: 0.999, y: 0.5 }, zoom: 3 }
    const once = visibleTiles(view, 600, 256, { repeat: false })
    expect(once.every((tile) => tile.column <= 7)).toBe(true)
    expect(Math.max(...once.map((tile) => tile.column))).toBe(7)
  })
})
