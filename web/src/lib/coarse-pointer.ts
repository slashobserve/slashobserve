import { useSyncExternalStore } from "react"

const coarsePointer = "(pointer: coarse)"

function subscribeCoarsePointer(onChange: () => void) {
  const media = window.matchMedia(coarsePointer)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

export function useCoarsePointer(): boolean {
  return useSyncExternalStore(
    subscribeCoarsePointer,
    () => window.matchMedia(coarsePointer).matches,
    () => false
  )
}
