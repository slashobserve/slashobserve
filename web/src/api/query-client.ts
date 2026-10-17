import { QueryClient } from "@tanstack/react-query"

import { ApiError, isRetryable } from "./errors"

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: (count, error) => count < 2 && isRetryable(error),
        retryDelay: (attempt, error) =>
          error instanceof ApiError && error.retryAfterMs !== undefined
            ? error.retryAfterMs
            : Math.min(1000 * 2 ** attempt, 8000),
      },
      mutations: { retry: false },
    },
  })
}
