import { useCallback, useEffect, useRef, useSyncExternalStore } from "react"

import { kioskStep } from "./kiosk"

function isFullscreen(): boolean {
  return Boolean(document.fullscreenElement)
}

function onFullscreenChange(listener: () => void): () => void {
  document.addEventListener("fullscreenchange", listener)
  return () => document.removeEventListener("fullscreenchange", listener)
}

function ignoreRefusal() {}

export function requestFullscreen(): void {
  if (!document.fullscreenEnabled) return
  document.documentElement.requestFullscreen().catch(ignoreRefusal)
}

export function useKioskFullscreen(kiosk: boolean, leaveKiosk: () => void) {
  const fullscreen = useSyncExternalStore(
    onFullscreenChange,
    isFullscreen,
    () => false
  )
  const latest = useRef({ kiosk, leaveKiosk })
  const leaving = useRef(false)
  useEffect(() => {
    latest.current = { kiosk, leaveKiosk }
  })
  useEffect(() => {
    leaving.current = false
    const step = kioskStep("kiosk-changed", {
      kiosk,
      fullscreen: isFullscreen(),
    })
    if (step === "exit-fullscreen")
      document.exitFullscreen().catch(ignoreRefusal)
  }, [kiosk])
  const leave = useCallback(() => {
    if (!latest.current.kiosk || leaving.current) return
    leaving.current = true
    latest.current.leaveKiosk()
  }, [])
  useEffect(
    () =>
      onFullscreenChange(() => {
        const step = kioskStep("fullscreen-changed", {
          kiosk: latest.current.kiosk,
          fullscreen: isFullscreen(),
        })
        if (step === "leave-kiosk") leave()
      }),
    [leave]
  )
  const offerFullscreen =
    kioskStep("fullscreen-key", { kiosk, fullscreen }) === "enter-fullscreen"
  return { offerFullscreen, leave }
}
