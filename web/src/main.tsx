import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClientProvider } from "@tanstack/react-query"
import { RouterProvider, createRouter } from "@tanstack/react-router"

import { setUnauthorizedHandler } from "@/api/client"
import { createQueryClient } from "@/api/query-client"
import { refreshSession } from "@/lib/auth"
import { parseSearch, stringifySearch } from "@/lib/search"
import { routeTree } from "./routeTree.gen"
import "./index.css"

const queryClient = createQueryClient()

const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  defaultPreloadStaleTime: 0,
  context: { queryClient },
  parseSearch,
  stringifySearch,
})

setUnauthorizedHandler(() => void router.invalidate())

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState !== "visible") return
  refreshSession().then(
    ({ changed }) => {
      if (changed) void router.invalidate()
    },
    () => undefined
  )
})

const rootElement = document.getElementById("root")
if (!rootElement) {
  throw new Error("missing #root element")
}

createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>
)
