import { useEffect, useRef, useState } from "react"

export function useDebouncedValue<T>(
  value: T,
  key: string,
  delayMs: number
): T {
  const [settled, setSettled] = useState(value)
  const latest = useRef(value)
  useEffect(() => {
    latest.current = value
  })
  useEffect(() => {
    const timer = setTimeout(() => setSettled(latest.current), delayMs)
    return () => clearTimeout(timer)
  }, [key, delayMs])
  return settled
}
