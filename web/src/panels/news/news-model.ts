import { ApiError } from "@/api/errors"

const maxRetries = 2

export function newsHref(link: string | null): string | null {
  if (link === null) return null
  try {
    const { protocol } = new URL(link)
    return protocol === "https:" || protocol === "http:" ? link : null
  } catch {
    return null
  }
}

export function isNotAllowed(error: unknown): boolean {
  return (
    error instanceof ApiError && error.code === "outbound_domain_not_allowed"
  )
}

export function retryNews(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
    return false
  }
  return failureCount < maxRetries
}
