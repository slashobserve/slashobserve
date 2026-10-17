import createClient, { type Middleware } from "openapi-fetch"

import { invalidateSession } from "@/lib/auth"
import { NetworkError } from "./errors"
import type { paths } from "./schema.gen"

let onUnauthorized: () => void = () => {}

export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

const unauthorized: Middleware = {
  onResponse({ response }) {
    if (response.status === 401) {
      invalidateSession()
      onUnauthorized()
    }
    return response
  },
}

async function fetchOrThrowNetworkError(
  request: Request,
  init?: RequestInit
): Promise<Response> {
  try {
    return await globalThis.fetch(request, init)
  } catch (cause) {
    if (cause instanceof TypeError) {
      throw new NetworkError(cause.message, { cause })
    }
    throw cause
  }
}

export const api = createClient<paths>({
  fetch: fetchOrThrowNetworkError,
  baseUrl: "/api/v1",
  credentials: "same-origin",
  headers: { Accept: "application/json" },
})
api.use(unauthorized)
