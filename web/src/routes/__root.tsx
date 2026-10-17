import { Link, Outlet, createRootRoute } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"

export const Route = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFound,
})

function RootLayout() {
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <Outlet />
    </div>
  )
}

function NotFound() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">Page not found</h1>
      <p className="text-sm text-muted-foreground">
        There is nothing at this address.
      </p>
      <Link to="/" className={buttonVariants({ variant: "outline" })}>
        Back to home
      </Link>
    </main>
  )
}
