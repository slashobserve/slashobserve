import { Link } from "@tanstack/react-router"

import { EmptyState } from "@/components/empty-state"
import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const frames = ["first", "second", "third"]

export function BuildWaiting() {
  return (
    <div className="flex flex-col gap-3">
      <div
        aria-hidden="true"
        className="grid grid-cols-2 gap-2.5 narrow:grid-cols-1"
      >
        {frames.map((frame) => (
          <div
            key={frame}
            className="h-[220px] rounded-card border border-dashed"
          />
        ))}
      </div>
      <p className="text-[13px] text-muted-foreground">
        The first panel appears here when the agent saves it.
      </p>
    </div>
  )
}

export function NothingSaved() {
  return (
    <EmptyState
      title="Nothing was saved."
      action={
        <Link
          to="/ask"
          className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
        >
          Back to Observe
        </Link>
      }
    />
  )
}
