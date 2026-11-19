import type { CSSProperties } from "react"

import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import { gridMargin } from "./fit-layout"
import { gridRowHeight } from "./stack-heights"

export function pageClass(stacked: boolean) {
  return cn(
    "flex overflow-x-clip bg-background text-foreground narrow:overflow-x-visible",
    stacked
      ? "min-h-svh flex-wrap"
      : "h-svh flex-nowrap narrow:h-auto narrow:min-h-svh"
  )
}

const skeletonPanels = [
  "min-[800px]:col-span-12 row-span-8",
  "min-[800px]:col-span-6 row-span-4",
  "min-[800px]:col-span-6 row-span-4",
  "min-[800px]:col-span-24 row-span-8",
]

export function DashboardPageSkeleton() {
  const open = useActivityStore((state) => state.open)
  const layout = useActivityLayout(false)
  return (
    <div className="flex min-h-svh bg-chrome" aria-busy="true">
      <div
        aria-hidden="true"
        className="sticky top-0 h-svh w-11 flex-none self-start bg-chrome narrow:hidden"
      />
      <div className={cn(pageClass(open && layout.stacked), "min-w-0 flex-1")}>
        <div className="@container/dash flex min-h-0 min-w-0 flex-[1_1_560px] flex-col">
          <div className="flex flex-col gap-y-2 pt-3 pr-2.5 pb-2.5 pl-4 @min-[900px]/dash:flex-row @min-[900px]/dash:items-center @min-[900px]/dash:justify-between @min-[900px]/dash:gap-x-4 narrow:flex-row narrow:items-center narrow:justify-between narrow:gap-x-4">
            <Skeleton className="h-[26px] w-64 animate-none motion-safe:animate-pulse" />
            <Skeleton className="h-8 w-[575px] max-w-full animate-none motion-safe:animate-pulse narrow:w-24" />
          </div>
          <main className="min-h-0 flex-1 overflow-auto pt-1 pr-2.5 pb-2.5 pl-4">
            <div
              className="grid grid-cols-1 min-[800px]:grid-cols-24"
              style={{ gridAutoRows: gridRowHeight, gap: gridMargin }}
            >
              {skeletonPanels.map((span, index) => (
                <Skeleton
                  key={index}
                  className={`${span} animate-none motion-safe:animate-pulse`}
                />
              ))}
            </div>
          </main>
        </div>
        {open ? (
          <div
            aria-hidden="true"
            style={{ "--activity-w": `${layout.width}px` } as CSSProperties}
            className={cn(
              "bg-background narrow:fixed narrow:inset-x-0 narrow:bottom-0 narrow:z-40 narrow:h-[85svh] narrow:w-auto narrow:rounded-t-card narrow:border-t narrow:border-l-0",
              layout.stacked
                ? "h-[70svh] w-full flex-none border-t"
                : "max-h-full max-w-full flex-[0_0_var(--activity-w)] border-l"
            )}
          />
        ) : null}
      </div>
    </div>
  )
}
