import { Link } from "@tanstack/react-router"

import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export function DashboardNotFound() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-2xl font-semibold">Dashboard not found</h1>
      <p className="text-sm text-muted-foreground">
        It may have been removed, or the link is wrong.
      </p>
      <Link to="/" className={cn(buttonVariants({ variant: "outline" }))}>
        Back to dashboards
      </Link>
    </main>
  )
}
