import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { RouterProvider, createRouter } from "@tanstack/react-router"

import { invalidateSession, refreshSession } from "@/lib/auth"
import { routeTree } from "./routeTree.gen"
import "./index.css"

const router = createRouter({ routeTree, defaultPreload: "intent" })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState !== "visible") return
  refreshSession().then(
    (session) => {
      if (session.authenticated) return
      invalidateSession()
      void router.invalidate()
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
    <RouterProvider router={router} />
  </StrictMode>
)
