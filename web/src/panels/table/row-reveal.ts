import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from "react"

type Pending = { key: string; among: readonly { key: string }[] }

export function useRowReveal(
  regionRef: RefObject<HTMLElement | null>,
  among: readonly { key: string }[]
): (key: string) => void {
  const pending = useRef<Pending | null>(null)
  const [, rerender] = useState(0)
  useLayoutEffect(() => {
    const want = pending.current
    const region = regionRef.current
    if (want === null || region === null) return
    if (want.among !== among) {
      pending.current = null
      return
    }
    const row = Array.from(
      region.querySelectorAll<HTMLElement>("[data-row-key]")
    ).find((element) => element.dataset.rowKey === want.key)
    if (row === undefined) return
    pending.current = null
    row.scrollIntoView({ block: "nearest" })
  })
  return useCallback(
    (key: string) => {
      if (!among.some((row) => row.key === key)) return
      pending.current = { key, among }
      rerender((count) => count + 1)
    },
    [among]
  )
}
