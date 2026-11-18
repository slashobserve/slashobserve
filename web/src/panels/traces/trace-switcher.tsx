import { Link } from "@tanstack/react-router"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"

import { Button, buttonVariants } from "@/components/ui/button"
import { traceLink } from "@/lib/links"
import { cn } from "@/lib/utils"

export const switcherHeight = 32

export function switcherLabel(index: number, count: number): string {
  return `Trace ${(index + 1).toLocaleString("en")} of ${count.toLocaleString("en")}`
}

export function literalDatasource(datasource: string): string | null {
  return datasource === "" || datasource.includes("$") ? null : datasource
}

type Props = {
  index: number
  count: number
  traceId: string
  datasource: string
  range: { from: string; to: string } | undefined
  onChange: (index: number) => void
}

export function TraceSwitcher({
  index,
  count,
  traceId,
  datasource,
  range,
  onChange,
}: Props) {
  const uid = literalDatasource(datasource)
  return (
    <div
      className="flex shrink-0 items-center gap-1 border-b border-border px-2 text-xs"
      style={{ height: switcherHeight }}
    >
      {count > 1 ? (
        <>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            aria-label="Previous trace"
            disabled={index === 0}
            onClick={() => onChange(index - 1)}
          >
            <ChevronLeftIcon />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            aria-label="Next trace"
            disabled={index === count - 1}
            onClick={() => onChange(index + 1)}
          >
            <ChevronRightIcon />
          </Button>
          <span className="text-muted-foreground tabular-nums">
            {switcherLabel(index, count)}
          </span>
        </>
      ) : null}
      {uid === null ? null : (
        <Link
          {...traceLink(traceId, uid, range)}
          className={cn(
            buttonVariants({ variant: "ghost", size: "xs" }),
            "ml-auto"
          )}
        >
          Open trace
        </Link>
      )}
    </div>
  )
}
