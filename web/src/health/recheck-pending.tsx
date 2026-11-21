import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"

import { CheckLine } from "./check-line"
import {
  recheckPendingLabel,
  recheckPendingText,
  type HealthItem,
} from "./health-labels"

export function RecheckPending({
  item,
  title,
  size,
  variant,
  align,
  className,
}: {
  item: HealthItem
  title?: string
  size: "sm" | "xs"
  variant: "ghost" | "outline"
  align: "start" | "end"
  className?: string
}) {
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant={variant}
            size={size}
            aria-haspopup="dialog"
            aria-label={
              title === undefined
                ? undefined
                : `Health of ${title}: ${recheckPendingLabel}`
            }
            className={className}
          />
        }
      >
        <StatusLabel tone="none">{recheckPendingLabel}</StatusLabel>
      </PopoverTrigger>
      <PopoverContent align={align} className="w-80">
        <PopoverTitle>Health</PopoverTitle>
        <p>{recheckPendingText}</p>
        <p className="text-xs text-muted-foreground">
          <CheckLine item={item} />
        </p>
      </PopoverContent>
    </Popover>
  )
}
