import { useSyncExternalStore } from "react"

const narrowQuery = "(max-width: 899px)"

function subscribeNarrow(onChange: () => void) {
  const media = window.matchMedia(narrowQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

export function useNarrow(): boolean {
  return useSyncExternalStore(
    subscribeNarrow,
    () => window.matchMedia(narrowQuery).matches,
    () => false
  )
}
