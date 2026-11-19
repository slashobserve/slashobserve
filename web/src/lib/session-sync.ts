import type { QueryClient } from "@tanstack/react-query"

import { refreshSession } from "./auth"

export function catchUpWithSession(
  router: { invalidate: () => Promise<void> },
  queryClient: QueryClient
) {
  refreshSession().then(
    ({ changed }) => {
      if (!changed) return
      void router.invalidate()
      void queryClient.invalidateQueries()
    },
    () => undefined
  )
}
