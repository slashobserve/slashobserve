import { Skeleton } from "@/components/ui/skeleton"

export function PanelSkeleton() {
  return (
    <div className="size-full p-2 pt-0">
      <Skeleton className="size-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}
