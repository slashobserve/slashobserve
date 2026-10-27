export type Point = { x: number; y: number }
export type Layout = Map<string, Point>
export type LayoutNode = { id: string; title: string }
export type LayoutEdge = { source: string; target: string }

type Adjacency = number[][]

function successorsOf(
  nodes: readonly LayoutNode[],
  edges: readonly LayoutEdge[]
): Adjacency {
  const index = new Map(nodes.map((node, at) => [node.id, at]))
  const out: Adjacency = nodes.map(() => [])
  for (const edge of edges) {
    const from = index.get(edge.source)
    const to = index.get(edge.target)
    if (from !== undefined && to !== undefined && from !== to) {
      out[from].push(to)
    }
  }
  return out
}

function withoutCycles(out: Adjacency): Adjacency {
  const active = 1
  const done = 2
  const state = new Uint8Array(out.length)
  const acyclic = out.map(() => new Set<number>())
  for (let root = 0; root < out.length; root++) {
    if (state[root] !== 0) continue
    state[root] = active
    const stack: [node: number, next: number][] = [[root, 0]]
    while (stack.length > 0) {
      const top = stack[stack.length - 1]
      const [node, next] = top
      if (next === out[node].length) {
        state[node] = done
        stack.pop()
        continue
      }
      top[1]++
      const to = out[node][next]
      if (state[to] === active) {
        acyclic[to].add(node)
        continue
      }
      acyclic[node].add(to)
      if (state[to] === 0) {
        state[to] = active
        stack.push([to, 0])
      }
    }
  }
  return acyclic.map((targets) => [...targets])
}

function longestPathLayers(out: Adjacency): number[] {
  const waiting = new Array<number>(out.length).fill(0)
  for (const targets of out) for (const to of targets) waiting[to]++
  const layers = new Array<number>(out.length).fill(0)
  const queue = waiting.flatMap((count, at) => (count === 0 ? [at] : []))
  for (let head = 0; head < queue.length; head++) {
    const from = queue[head]
    for (const to of out[from]) {
      layers[to] = Math.max(layers[to], layers[from] + 1)
      if (--waiting[to] === 0) queue.push(to)
    }
  }
  return layers
}

function reversed(out: Adjacency): Adjacency {
  const back: Adjacency = out.map(() => [])
  out.forEach((targets, from) => {
    for (const to of targets) back[to].push(from)
  })
  return back
}

function orderWithinLayers(
  layers: readonly number[],
  out: Adjacency
): number[][] {
  const rows: number[][] = []
  layers.forEach((layer, node) => {
    while (rows.length <= layer) rows.push([])
    rows[layer].push(node)
  })
  const height = new Float64Array(layers.length)
  const center = (row: number[]) => {
    row.forEach((node, at) => {
      height[node] = at - (row.length - 1) / 2
    })
  }
  rows.forEach(center)

  const reorder = (row: number[], neighbours: Adjacency) => {
    const keys = new Map(
      row.map((node) => {
        const near = neighbours[node]
        const key =
          near.length === 0
            ? height[node]
            : near.reduce((sum, other) => sum + height[other], 0) / near.length
        return [node, key]
      })
    )
    row.sort((a, b) => (keys.get(a) ?? 0) - (keys.get(b) ?? 0))
    center(row)
  }

  const incoming = reversed(out)
  for (let layer = 1; layer < rows.length; layer++)
    reorder(rows[layer], incoming)
  for (let layer = rows.length - 2; layer >= 0; layer--) {
    reorder(rows[layer], out)
  }
  return rows
}

export function layeredLayout(
  nodes: readonly LayoutNode[],
  edges: readonly LayoutEdge[]
): Layout {
  const acyclic = withoutCycles(successorsOf(nodes, edges))
  const layers = longestPathLayers(acyclic)
  const rows = orderWithinLayers(layers, acyclic)
  const positions: Layout = new Map()
  rows.forEach((row, layer) => {
    row.forEach((node, at) => {
      positions.set(nodes[node].id, { x: layer, y: at - (row.length - 1) / 2 })
    })
  })
  return positions
}

function compareText(a: string, b: string): number {
  if (a === b) return 0
  return a < b ? -1 : 1
}

export function gridLayout(nodes: readonly LayoutNode[]): Layout {
  const columns = Math.ceil(Math.sqrt(nodes.length))
  const sorted = [...nodes].sort(
    (a, b) => compareText(a.title, b.title) || compareText(a.id, b.id)
  )
  return new Map(
    sorted.map((node, at) => [
      node.id,
      { x: at % columns, y: Math.floor(at / columns) },
    ])
  )
}
