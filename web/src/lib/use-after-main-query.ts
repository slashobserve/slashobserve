import { useState } from "react"

import { ApiError } from "@/api/errors"

export interface MainQueryState {
  isSuccess: boolean
  isError: boolean
  isPlaceholderData: boolean
  error: unknown
}

export function timedOut(main: Pick<MainQueryState, "isError" | "error">) {
  return (
    main.isError &&
    main.error instanceof ApiError &&
    main.error.code === "query_timeout"
  )
}

export function useAfterMainQuery(
  main: MainQueryState,
  scope: string
): { ready: boolean; slow: boolean } {
  const [started, setStarted] = useState<string | null>(null)
  const slow = timedOut(main)
  const answered = main.isSuccess && !main.isPlaceholderData
  const done = answered || (main.isError && !slow)
  if (done && started !== scope) setStarted(scope)
  const ready = done || started === scope
  return { ready, slow: slow && !ready }
}
