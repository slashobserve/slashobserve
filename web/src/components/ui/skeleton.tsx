import { cn } from "cn"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn(
        "rounded-md bg-foreground/8 motion-safe:animate-pulse dark:bg-muted",
        className
      )}
      {...props}
    />
  )
}

export { Skeleton }
