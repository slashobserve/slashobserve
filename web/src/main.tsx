import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClientProvider } from "@tanstack/react-query"
import { RouterProvider, createRouter } from "@tanstack/react-router"

import { watchPostedDrafts } from "@/activity/activity-store"
import { setRoleRefusedHandler, setUnauthorizedHandler } from "@/api/client"
import { createQueryClient } from "@/api/query-client"
import { parseSearch, stringifySearch } from "@/lib/search"
import { followNavigationWithInbox } from "@/inbox/inbox-panel-store"
import { catchUpWithSession } from "@/lib/session-sync"
import { followTraceOrigin } from "@/traces/trace-origin"
import { routeTree } from "./routeTree.gen"
import "./index.css"

const queryClient = createQueryClient()
watchPostedDrafts(queryClient)

const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  defaultPreloadStaleTime: 0,
  context: { queryClient },
  parseSearch,
  stringifySearch,
})

setUnauthorizedHandler(() => void router.invalidate())
followTraceOrigin(router)
followNavigationWithInbox(router)

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

function catchUp() {
  catchUpWithSession(router, queryClient)
}

setRoleRefusedHandler(catchUp)

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") catchUp()
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
