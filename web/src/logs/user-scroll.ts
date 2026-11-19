import { useCallback, useEffect, useMemo, useRef } from "react"

export const userScrollWindowMs = 1000

export type UserScroll = {
  handlers: {
    onWheelCapture: () => void
    onTouchMoveCapture: () => void
    onKeyDownCapture: () => void
    onPointerDownCapture: () => void
  }
  recent: () => boolean
}

export function useUserScroll(now: () => number = Date.now): UserScroll {
  const lastInput = useRef<number | null>(null)
  const held = useRef(false)

  const touch = useCallback(() => {
    lastInput.current = now()
  }, [now])

  useEffect(() => {
    function release() {
      held.current = false
    }
    document.addEventListener("pointerup", release)
    document.addEventListener("pointercancel", release)
    return () => {
      document.removeEventListener("pointerup", release)
      document.removeEventListener("pointercancel", release)
    }
  }, [])

  return useMemo(
    () => ({
      handlers: {
        onWheelCapture: touch,
        onTouchMoveCapture: touch,
        onKeyDownCapture: touch,
        onPointerDownCapture: () => {
          held.current = true
          touch()
        },
      },
      recent: () =>
        held.current ||
        (lastInput.current !== null &&
          now() - lastInput.current <= userScrollWindowMs),
    }),
    [touch, now]
  )
}
