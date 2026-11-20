import { useSearch } from "@tanstack/react-router"

import { validObservation } from "@/logs/search"

export function useObservationOrigin(): string | undefined {
  const search: Record<string, unknown> = useSearch({ strict: false })
  return validObservation(search.observation)
}
