import { Skeleton } from "@/components/ui/skeleton"

export function PublicPageSkeleton() {
  return (
    <div
      className="flex flex-col gap-3 pt-3 pr-2.5 pb-2.5 pl-4"
      aria-busy="true"
    >
      <Skeleton className="h-[30px] w-64 animate-none motion-safe:animate-pulse" />
      <Skeleton className="h-64 w-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}
