import type { Layout, LayoutEdge, LayoutNode } from "./layout"

export type ForceArea = { width: number; height: number; seed: number }

const minIterations = 50
const maxIterations = 300
const treeThreshold = 200
const openingAngle = 0.8
const maxTreeDepth = 32
const minDistance = 0.01
const startingHeat = 0.1
const gravity = 0.5

function seededRandom(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let mixed = Math.imul(state ^ (state >>> 15), state | 1)
    mixed ^= mixed + Math.imul(mixed ^ (mixed >>> 7), mixed | 61)
    return ((mixed ^ (mixed >>> 14)) >>> 0) / 4294967296
  }
}

function connectedPairs(
  nodes: readonly LayoutNode[],
  edges: readonly LayoutEdge[]
): [number, number][] {
  const index = new Map(nodes.map((node, at) => [node.id, at]))
  const seen = new Set<number>()
  const pairs: [number, number][] = []
  for (const edge of edges) {
    const from = index.get(edge.source)
    const to = index.get(edge.target)
    if (from === undefined || to === undefined || from === to) continue
    const key = Math.min(from, to) * nodes.length + Math.max(from, to)
    if (seen.has(key)) continue
    seen.add(key)
    pairs.push([from, to])
  }
  return pairs
}

class QuadTree {
  private readonly capacity: number
  private readonly left: Float64Array
  private readonly top: Float64Array
  private readonly size: Float64Array
  private readonly sumX: Float64Array
  private readonly sumY: Float64Array
  private readonly count: Int32Array
  private readonly body: Int32Array
  private readonly kids: Int32Array
  private used = 0

  private readonly xs: Float64Array
  private readonly ys: Float64Array

  constructor(xs: Float64Array, ys: Float64Array, side: number) {
    this.xs = xs
    this.ys = ys
    this.capacity = 4 * xs.length + 64
    this.left = new Float64Array(this.capacity)
    this.top = new Float64Array(this.capacity)
    this.size = new Float64Array(this.capacity)
    this.sumX = new Float64Array(this.capacity)
    this.sumY = new Float64Array(this.capacity)
    this.count = new Int32Array(this.capacity)
    this.body = new Int32Array(this.capacity).fill(-1)
    this.kids = new Int32Array(this.capacity * 4).fill(-1)
    this.newCell(0, 0, side)
    for (let i = 0; i < xs.length; i++) this.insert(i)
  }

  private newCell(left: number, top: number, size: number): number {
    const cell = this.used++
    this.left[cell] = left
    this.top[cell] = top
    this.size[cell] = size
    return cell
  }

  private quadrant(cell: number, x: number, y: number): number {
    const half = this.size[cell] / 2
    return (
      (x >= this.left[cell] + half ? 1 : 0) +
      (y >= this.top[cell] + half ? 2 : 0)
    )
  }

  private childFor(cell: number, x: number, y: number): number {
    const slot = cell * 4 + this.quadrant(cell, x, y)
    if (this.kids[slot] !== -1) return this.kids[slot]
    if (this.used === this.capacity) return -1
    const half = this.size[cell] / 2
    const q = slot - cell * 4
    this.kids[slot] = this.newCell(
      this.left[cell] + (q % 2) * half,
      this.top[cell] + Math.floor(q / 2) * half,
      half
    )
    return this.kids[slot]
  }

  private holds(cell: number, x: number, y: number): boolean {
    return (
      x >= this.left[cell] &&
      x <= this.left[cell] + this.size[cell] &&
      y >= this.top[cell] &&
      y <= this.top[cell] + this.size[cell]
    )
  }

  private isLeaf(cell: number): boolean {
    return this.body[cell] !== -1
  }

  private add(cell: number, point: number) {
    this.sumX[cell] += this.xs[point]
    this.sumY[cell] += this.ys[point]
    this.count[cell]++
  }

  private insert(point: number) {
    const x = this.xs[point]
    const y = this.ys[point]
    let cell = 0
    for (let depth = 0; ; depth++) {
      if (this.count[cell] === 0) {
        this.add(cell, point)
        this.body[cell] = point
        return
      }
      if (this.isLeaf(cell)) {
        const resident = this.body[cell]
        const room =
          depth < maxTreeDepth
            ? this.childFor(cell, this.xs[resident], this.ys[resident])
            : -1
        if (room === -1) {
          this.add(cell, point)
          return
        }
        this.body[cell] = -1
        this.add(room, resident)
        this.body[room] = resident
      }
      this.add(cell, point)
      const next = this.childFor(cell, x, y)
      if (next === -1) {
        this.body[cell] = point
        return
      }
      cell = next
    }
  }

  repulsion(
    point: number,
    strength: number,
    stack: Int32Array
  ): [number, number] {
    const x = this.xs[point]
    const y = this.ys[point]
    let forceX = 0
    let forceY = 0
    let top = 0
    stack[top++] = 0
    while (top > 0) {
      const cell = stack[--top]
      const mass = this.count[cell]
      if (mass === 0 || this.body[cell] === point) continue
      const dx = x - this.sumX[cell] / mass
      const dy = y - this.sumY[cell] / mass
      const distance = Math.max(Math.hypot(dx, dy), minDistance)
      const far =
        this.size[cell] / distance < openingAngle && !this.holds(cell, x, y)
      if (this.isLeaf(cell) || far) {
        const push = (strength * mass) / (distance * distance)
        forceX += dx * push
        forceY += dy * push
        continue
      }
      for (let q = 0; q < 4; q++) {
        const kid = this.kids[cell * 4 + q]
        if (kid !== -1) stack[top++] = kid
      }
    }
    return [forceX, forceY]
  }
}

export function forceLayout(
  nodes: readonly LayoutNode[],
  edges: readonly LayoutEdge[],
  { width, height, seed }: ForceArea
): Layout {
  const count = nodes.length
  const layout: Layout = new Map()
  if (count === 0) return layout
  if (count === 1) {
    layout.set(nodes[0].id, { x: width / 2, y: height / 2 })
    return layout
  }

  const random = seededRandom(seed)
  const xs = new Float64Array(count)
  const ys = new Float64Array(count)
  for (let i = 0; i < count; i++) {
    xs[i] = random() * width
    ys[i] = random() * height
  }

  const pairs = connectedPairs(nodes, edges)
  const ideal = Math.max(Math.sqrt((width * height) / count), 1)
  const repulsionStrength = ideal * ideal
  const side = Math.max(width, height)
  const iterations = Math.min(maxIterations, minIterations + count)
  const dispX = new Float64Array(count)
  const dispY = new Float64Array(count)
  const stack = new Int32Array(4 * maxTreeDepth * 4 + 16)

  for (let step = 0; step < iterations; step++) {
    const heat = side * startingHeat * (1 - step / iterations)
    dispX.fill(0)
    dispY.fill(0)

    if (count > treeThreshold) {
      const tree = new QuadTree(xs, ys, side)
      for (let i = 0; i < count; i++) {
        const [fx, fy] = tree.repulsion(i, repulsionStrength, stack)
        dispX[i] += fx
        dispY[i] += fy
      }
    } else {
      for (let i = 0; i < count; i++) {
        for (let j = i + 1; j < count; j++) {
          const dx = xs[i] - xs[j]
          const dy = ys[i] - ys[j]
          const distance = Math.max(Math.hypot(dx, dy), minDistance)
          const push = repulsionStrength / (distance * distance)
          dispX[i] += dx * push
          dispY[i] += dy * push
          dispX[j] -= dx * push
          dispY[j] -= dy * push
        }
      }
    }

    for (const [from, to] of pairs) {
      const dx = xs[from] - xs[to]
      const dy = ys[from] - ys[to]
      const distance = Math.max(Math.hypot(dx, dy), minDistance)
      const pull = distance / ideal
      dispX[from] -= dx * pull
      dispY[from] -= dy * pull
      dispX[to] += dx * pull
      dispY[to] += dy * pull
    }

    for (let i = 0; i < count; i++) {
      dispX[i] -= (xs[i] - width / 2) * gravity
      dispY[i] -= (ys[i] - height / 2) * gravity
      const length = Math.hypot(dispX[i], dispY[i])
      if (length === 0) continue
      const move = Math.min(length, heat) / length
      xs[i] += dispX[i] * move
      ys[i] += dispY[i] * move
    }
  }

  nodes.forEach((node, at) => layout.set(node.id, { x: xs[at], y: ys[at] }))
  return layout
}
