type CursorSync = "off" | "crosshair" | "tooltip"
type PeerCursor = { x: number; tooltip: boolean } | null
export type CursorMember = {
  move: (x: number | null) => void
  leave: () => void
}
export type SharedCursor = {
  join: (draw: (peer: PeerCursor) => void) => CursorMember
}
export type SharedCursorHub = SharedCursor & {
  setMode: (mode: CursorSync) => void
}

export function createSharedCursor(): SharedCursorHub {
  const members = new Map<symbol, (peer: PeerCursor) => void>()
  let mode: CursorSync = "off"
  let owner: symbol | null = null
  let x: number | null = null

  function peer(): PeerCursor {
    if (mode === "off" || owner === null || x === null) return null
    return { x, tooltip: mode === "tooltip" }
  }

  function broadcast() {
    const current = peer()
    for (const [id, draw] of members) {
      if (id !== owner) draw(current)
    }
  }

  function clear() {
    owner = null
    x = null
    if (mode !== "off") broadcast()
  }

  return {
    join(draw) {
      const id = Symbol("member")
      members.set(id, draw)
      const current = peer()
      if (current) draw(current)
      return {
        move(next) {
          if (!members.has(id)) return
          if (next === null) {
            if (owner === id) clear()
            return
          }
          owner = id
          x = next
          if (mode !== "off") broadcast()
        },
        leave() {
          if (!members.delete(id)) return
          if (owner === id) clear()
        },
      }
    },
    setMode(next) {
      if (next === mode) return
      mode = next
      broadcast()
    },
  }
}

export const noSharedCursor: SharedCursor = {
  join: () => ({ move: () => {}, leave: () => {} }),
}
