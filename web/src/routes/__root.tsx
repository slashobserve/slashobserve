import type { ReactNode } from "react"
import type { QueryClient } from "@tanstack/react-query"
import {
  Link,
  Outlet,
  createRootRouteWithContext,
  useRouter,
} from "@tanstack/react-router"

import { Button, buttonVariants } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { cn } from "cn"

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  component: RootLayout,
  notFoundComponent: NotFound,
  errorComponent: RootError,
})

function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      {children}
    </div>
  )
}

function RootLayout() {
  return (
    <Shell>
      <TooltipProvider>
        <Outlet />
      </TooltipProvider>
      <Toaster position="bottom-right" />
    </Shell>
  )
}

function NotFound() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">Page not found</h1>
      <p className="text-sm text-muted-foreground">
        There is nothing at this address.
      </p>
      <Link to="/" className={cn(buttonVariants({ variant: "outline" }))}>
        Back to home
      </Link>
    </main>
  )
}

function RootError({ reset }: { reset: () => void }) {
  const router = useRouter()
  return (
    <Shell>
      <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
        <h1 className="text-2xl font-semibold">Something went wrong</h1>
        <p className="text-sm text-muted-foreground">
          The page could not be loaded.
        </p>
        <Button
          variant="outline"
          onClick={() => {
            reset()
            void router.invalidate()
          }}
        >
          Retry
        </Button>
      </main>
    </Shell>
  )
}
