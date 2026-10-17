import createClient, { type Middleware } from "openapi-fetch"

import { invalidateSession } from "@/lib/auth"
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

export const api = createClient<paths>({
  baseUrl: "/api/v1",
  credentials: "same-origin",
  headers: { Accept: "application/json" },
})
api.use(unauthorized)
