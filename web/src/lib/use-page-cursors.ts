import { useCallback, useState } from "react"

type Chain = { scopeKey: string; cursors: ReadonlyMap<number, string> }

const noCursors: ReadonlyMap<number, string> = new Map()

export function usePageCursors(scopeKey: string, byOffset: boolean) {
  const [chain, setChain] = useState<Chain>({
    scopeKey,
    cursors: noCursors,
  })
  const cursors = chain.scopeKey === scopeKey ? chain.cursors : noCursors

  const remember = useCallback(
    (page: number, next: string | undefined) => {
      if (next === undefined) return
      setChain((current) => {
        const known =
          current.scopeKey === scopeKey ? current.cursors : noCursors
        if (known.get(page + 1) === next) return current
        return { scopeKey, cursors: new Map(known).set(page + 1, next) }
      })
    },
    [scopeKey]
  )

  const cursorFor = (page: number): string | undefined => cursors.get(page)

  const reachable = (page: number): boolean =>
    page === 1 || byOffset || cursors.has(page)

  return { remember, cursorFor, reachable }
}
