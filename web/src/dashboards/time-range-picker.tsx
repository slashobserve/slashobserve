import { useId, useState, type FormEvent } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { ClockIcon, RefreshCwIcon } from "lucide-react"

import { dashboardKeys } from "@/api/keys"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  describeRange,
  formatTimeValue,
  isAbsoluteOrdered,
  parseUserTime,
  quickRanges,
} from "@/lib/time"

import { useSetDashboardSearch } from "./search"

type Props = {
  dashboardId: string
  version: number
  from: string
  to: string
}

export function TimeRangePicker({ dashboardId, version, from, to }: Props) {
  const queryClient = useQueryClient()
  const setSearch = useSetDashboardSearch()
  const fromId = useId()
  const toId = useId()
  const [open, setOpen] = useState(false)
  const [draftFrom, setDraftFrom] = useState("")
  const [draftTo, setDraftTo] = useState("")
  const [error, setError] = useState<string | null>(null)

  function choose(nextFrom: string, nextTo: string) {
    setSearch((state) => ({ ...state, from: nextFrom, to: nextTo }))
    setOpen(false)
  }

  function onOpenChange(next: boolean) {
    if (next) {
      setDraftFrom(formatTimeValue(from))
      setDraftTo(formatTimeValue(to))
      setError(null)
    }
    setOpen(next)
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const parsedFrom = parseUserTime(draftFrom)
    const parsedTo = parseUserTime(draftTo)
    if (parsedFrom === null || parsedTo === null) {
      setError("Enter a time like now-6h, now/d or 2026-10-17 09:00.")
      return
    }
    if (!isAbsoluteOrdered(parsedFrom, parsedTo)) {
      setError("From must be earlier than To.")
      return
    }
    choose(parsedFrom, parsedTo)
  }

  const rangeLabel = describeRange(from, to)

  return (
    <div className="flex items-center gap-1">
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger
          render={
            <Button
              variant="outline"
              className="h-[30px]"
              aria-label={`Time range: ${rangeLabel}`}
            />
          }
        >
          <ClockIcon />
          {rangeLabel}
        </PopoverTrigger>
        <PopoverContent
          align="end"
          className="w-[min(34rem,calc(100vw-1rem))] gap-3 p-3"
        >
          <PopoverTitle className="sr-only">Time range</PopoverTitle>
          <div className="flex flex-col gap-3 sm:flex-row">
            <div
              role="group"
              aria-label="Quick ranges"
              className="grid grid-cols-2 gap-1 sm:flex sm:w-44 sm:shrink-0 sm:flex-col sm:gap-0 sm:border-r sm:pr-2"
            >
              {quickRanges.map((range) => (
                <Button
                  key={range.label}
                  variant="ghost"
                  size="sm"
                  className="justify-start aria-pressed:bg-muted"
                  aria-pressed={range.from === from && range.to === to}
                  onClick={() => choose(range.from, range.to)}
                >
                  {range.label}
                </Button>
              ))}
            </div>
            <form onSubmit={onSubmit} className="flex flex-1 flex-col gap-2">
              <div className="flex flex-col gap-1">
                <Label htmlFor={fromId}>From</Label>
                <Input
                  id={fromId}
                  value={draftFrom}
                  onChange={(event) => setDraftFrom(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label htmlFor={toId}>To</Label>
                <Input
                  id={toId}
                  value={draftTo}
                  onChange={(event) => setDraftTo(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <PopoverDescription className="text-xs">
                Use now-6h, now/d or 2026-10-17 09:00
              </PopoverDescription>
              {error ? (
                <p role="alert" className="text-xs text-destructive">
                  {error}
                </p>
              ) : null}
              <Button type="submit" className="self-start">
                Apply
              </Button>
            </form>
          </div>
        </PopoverContent>
      </Popover>
      <Button
        variant="outline"
        size="icon"
        className="size-[30px]"
        aria-label="Refresh now"
        onClick={() =>
          void queryClient.invalidateQueries({
            queryKey: dashboardKeys.panels(dashboardId, version),
          })
        }
      >
        <RefreshCwIcon />
      </Button>
    </div>
  )
}
