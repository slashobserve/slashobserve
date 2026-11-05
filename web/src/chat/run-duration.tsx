import { useEffect, useState } from "react"

import { formatRunDuration, turnDurationMs } from "./turn-model"

const tickMs = 1000

function useSecondClock(): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), tickMs)
    return () => clearInterval(timer)
  }, [])
  return now
}

export function RunningFor({ startedAt }: { startedAt: string | null }) {
  const now = useSecondClock()
  const ms = turnDurationMs(startedAt, null, now)
  if (ms === null) return null
  return (
    <p className="text-[13px] text-muted-foreground">
      Running for {formatRunDuration(ms)}
    </p>
  )
}
