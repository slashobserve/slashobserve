import type { ComponentProps, CSSProperties, ReactNode, Ref } from "react"
import { cn } from "@/lib/utils"

type TableCardName =
  { label: string; labelledBy?: never } | { labelledBy: string; label?: never }

type TableCardProps = TableCardName & {
  minWidth: number | string
  children: ReactNode
  className?: string
  ref?: Ref<HTMLDivElement>
  stackOnNarrow?: boolean
}

const stackedOnNarrow = [
  "min-w-(--table-min) narrow:block narrow:min-w-0",
  "narrow:[&_thead]:sr-only narrow:[&_tbody]:block",
  "narrow:[&_tbody_tr]:flex narrow:[&_tbody_tr]:flex-wrap narrow:[&_tbody_tr]:items-center narrow:[&_tbody_tr]:gap-x-3 narrow:[&_tbody_tr]:gap-y-1 narrow:[&_tbody_tr]:border-b narrow:[&_tbody_tr]:px-3.5 narrow:[&_tbody_tr]:py-3 narrow:[&_tbody_tr:last-child]:border-b-0",
  "narrow:[&_tbody_td]:min-w-0 narrow:[&_tbody_td]:border-0 narrow:[&_tbody_td]:p-0",
  'narrow:[&_tbody_td[data-label]]:flex narrow:[&_tbody_td[data-label]]:gap-1 narrow:[&_tbody_td[data-label]]:before:content-[attr(data-label)_":"]',
  "narrow:[&_tbody_td:empty:not(:first-child)]:hidden",
  "narrow:[&_tbody_td:first-child]:basis-full narrow:[&_tbody_td:first-child]:font-medium",
  "narrow:[&_tbody_td:not(:first-child):not([data-cell=action])]:text-xs narrow:[&_tbody_td:not(:first-child):not([data-cell=action])]:text-muted-foreground",
  "narrow:[&_tbody_td[data-cell=action]]:ml-auto",
].join(" ")

function cssLength(value: number | string) {
  return typeof value === "number" ? `${value}px` : value
}

export function TableCard({
  minWidth,
  label,
  labelledBy,
  children,
  className,
  ref,
  stackOnNarrow = false,
}: TableCardProps) {
  return (
    <div
      ref={ref}
      role="region"
      tabIndex={0}
      aria-label={label}
      aria-labelledby={labelledBy}
      className={cn(
        "relative overflow-x-auto rounded-card border bg-background outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className
      )}
    >
      <table
        className={cn(
          "w-full border-collapse text-[13px]",
          stackOnNarrow && stackedOnNarrow
        )}
        style={
          stackOnNarrow
            ? ({ "--table-min": cssLength(minWidth) } as CSSProperties)
            : { minWidth }
        }
      >
        {children}
      </table>
    </div>
  )
}

export function HeaderCell({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      scope="col"
      className={cn(
        "border-b px-3.5 py-[9px] text-left font-medium whitespace-nowrap text-muted-foreground",
        className
      )}
      {...props}
    />
  )
}

export function Cell({ className, ...props }: ComponentProps<"td">) {
  return (
    <td
      className={cn(
        "border-b px-3.5 py-[11px] align-middle [tr:last-child_&]:border-b-0",
        className
      )}
      {...props}
    />
  )
}
