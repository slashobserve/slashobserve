import { useEffect, useState } from "react"

const hintMs = 3000

export function KioskHint({ offerFullscreen }: { offerFullscreen: boolean }) {
  const [visible, setVisible] = useState(true)
  useEffect(() => {
    let timer = window.setTimeout(() => setVisible(false), hintMs)
    function onMove() {
      setVisible(true)
      window.clearTimeout(timer)
      timer = window.setTimeout(() => setVisible(false), hintMs)
    }
    window.addEventListener("mousemove", onMove)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener("mousemove", onMove)
    }
  }, [])
  return (
    <div
      role="status"
      aria-live="polite"
      className="pointer-events-none fixed inset-x-0 bottom-4 z-20 flex justify-center"
    >
      {visible ? (
        <p className="rounded-control border bg-background px-3 py-1.5 text-[13px] text-foreground">
          Press{" "}
          {offerFullscreen ? (
            <>
              <kbd className="rounded-sm border px-1">F</kbd> for full screen
              and{" "}
            </>
          ) : null}
          <kbd className="rounded-sm border px-1">Esc</kbd> to exit TV mode
        </p>
      ) : null}
    </div>
  )
}
