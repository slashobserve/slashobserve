import { useSyncExternalStore } from "react"

const foldBelow = 1200
const query = `(max-width: ${foldBelow - 1}px)`

export function foldedFor(width: number): boolean {
  return width < foldBelow
}

function subscribe(onChange: () => void) {
  const list = window.matchMedia(query)
  list.addEventListener("change", onChange)
  return () => list.removeEventListener("change", onChange)
}

function read() {
  return window.matchMedia(query).matches
}

export function useFoldedByWidth(): boolean {
  return useSyncExternalStore(subscribe, read, () => false)
}
