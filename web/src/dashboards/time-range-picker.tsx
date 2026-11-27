import { useId, useRef, useState, type FormEvent, type Ref } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { CalendarIcon, ClockIcon, RefreshCwIcon } from "lucide-react"
import { toast } from "sonner"

import { loadProblemText } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { DateCalendar } from "@/components/date-calendar"
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
import { pickDay, type DaySelection } from "@/lib/calendar"
import {
  describeRange,
  displayZone,
  quickRanges as defaultQuickRanges,
  type QuickRange,
  timeFormatHint,
  zonedTimeValue,
} from "@/lib/time"
import { preferredDisplayZone, usePreferences } from "@/lib/preferences"
import type { Dashboard } from "@/spec"

import { useAddVersion } from "./queries"
import { saveToast } from "./save-outcome"
import { useSetDashboardSearch } from "./search"
import {
  calendarMonthOf,
  readDraft,
  type DraftField,
  type DraftProblem,
} from "./time-draft"

export type TimeRangeValue = { from: string; to: string }

type SaveDefault = {
  saved: TimeRangeValue
  pending: boolean
  onSave: (next: TimeRangeValue) => void
}

type ControlProps = {
  value: TimeRangeValue
  onChange: (next: TimeRangeValue) => void
  quickRanges?: QuickRange[]
  zone?: string | null
  saveDefault?: SaveDefault
  defaultOpen?: boolean
  onDismiss?: () => void
}

export function TimeRangeControl({
  value,
  onChange,
  quickRanges = defaultQuickRanges,
  zone,
  saveDefault,
  defaultOpen = false,
  onDismiss,
}: ControlProps) {
  const preferredTimeZone = usePreferences((state) => state.timeZone)
  const typedZone =
    zone === undefined
      ? preferredDisplayZone(preferredTimeZone)
      : (zone ?? undefined)
  const fromId = useId()
  const toId = useId()
  const fromRef = useRef<HTMLInputElement>(null)
  const toRef = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(defaultOpen)
  const [draftFrom, setDraftFrom] = useState(() =>
    defaultOpen ? zonedTimeValue(value.from, typedZone) : ""
  )
  const [draftTo, setDraftTo] = useState(() =>
    defaultOpen ? zonedTimeValue(value.to, typedZone) : ""
  )
  const [problem, setProblem] = useState<DraftProblem | null>(null)
  const [calendarOpen, setCalendarOpen] = useState(false)
  const [picked, setPicked] = useState<DaySelection>(null)

  function choose(nextFrom: string, nextTo: string) {
    onChange({ from: nextFrom, to: nextTo })
    setOpen(false)
  }

  function onOpenChange(next: boolean) {
    if (next) {
      setDraftFrom(zonedTimeValue(value.from, typedZone))
      setDraftTo(zonedTimeValue(value.to, typedZone))
      setProblem(null)
      setCalendarOpen(false)
      setPicked(null)
    } else {
      onDismiss?.()
    }
    setOpen(next)
  }

  function read(): TimeRangeValue | null {
    const result = readDraft(draftFrom, draftTo, typedZone)
    if (result.range === undefined) {
      setProblem(result.problem)
      const field = result.problem.field === "from" ? fromRef : toRef
      field.current?.focus()
      return null
    }
    return result.range
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const range = read()
    if (range !== null) choose(range.from, range.to)
  }

  function onSaveDefault() {
    const range = read()
    if (range === null || saveDefault === undefined) return
    choose(range.from, range.to)
    saveDefault.onSave(range)
  }

  function onPickDay(day: string) {
    const next = pickDay(picked, day)
    setPicked(next)
    if (next === null) return
    setDraftFrom(`${next.start} 00:00`)
    setDraftTo(`${next.end ?? next.start} 23:59:59`)
    setProblem(null)
  }

  const rangeLabel = describeRange(value.from, value.to, quickRanges, typedZone)
  const parsed = readDraft(draftFrom, draftTo, typedZone).range
  const alreadyDefault =
    saveDefault !== undefined &&
    parsed !== undefined &&
    parsed.from === saveDefault.saved.from &&
    parsed.to === saveDefault.saved.to

  const fieldProblem = (field: DraftField) =>
    problem?.field === field ? problem.message : null

  return (
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
        className="max-h-[min(40rem,calc(100svh-4rem))] w-[min(34rem,calc(100vw-1rem))] gap-3 overflow-auto p-3"
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
                aria-pressed={
                  range.from === value.from && range.to === value.to
                }
                onClick={() => choose(range.from, range.to)}
              >
                {range.label}
              </Button>
            ))}
          </div>
          <form onSubmit={onSubmit} className="flex flex-1 flex-col gap-2">
            <TimeField
              id={fromId}
              label="From"
              value={draftFrom}
              inputRef={fromRef}
              problem={fieldProblem("from")}
              onChange={setDraftFrom}
            />
            <TimeField
              id={toId}
              label="To"
              value={draftTo}
              inputRef={toRef}
              problem={fieldProblem("to")}
              onChange={setDraftTo}
            />
            <PopoverDescription className="text-xs">
              Use {timeFormatHint(typedZone)}
            </PopoverDescription>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="self-start"
              aria-expanded={calendarOpen}
              onClick={() => setCalendarOpen((current) => !current)}
            >
              <CalendarIcon />
              Pick dates
            </Button>
            {calendarOpen ? (
              <DateCalendar
                initial={calendarMonthOf(draftFrom, typedZone)}
                selection={picked}
                onPick={onPickDay}
              />
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button type="submit">Apply</Button>
              {saveDefault === undefined ? null : (
                <Button
                  type="button"
                  variant="outline"
                  disabled={saveDefault.pending || alreadyDefault}
                  aria-busy={saveDefault.pending}
                  title={
                    alreadyDefault
                      ? "This is already the dashboard's default range"
                      : "Apply this range and save it as the dashboard's default in a new version"
                  }
                  onClick={onSaveDefault}
                >
                  Save as default
                </Button>
              )}
            </div>
          </form>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function TimeField({
  id,
  label,
  value,
  inputRef,
  problem,
  onChange,
}: {
  id: string
  label: string
  value: string
  inputRef: Ref<HTMLInputElement>
  problem: string | null
  onChange: (next: string) => void
}) {
  const errorId = `${id}-error`
  return (
    <div className="flex flex-col gap-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        ref={inputRef}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        autoComplete="off"
        spellCheck={false}
        aria-invalid={problem !== null}
        aria-describedby={problem === null ? undefined : errorId}
      />
      {problem === null ? null : (
        <p id={errorId} role="alert" className="text-xs text-destructive">
          {problem}
        </p>
      )}
    </div>
  )
}

type Props = {
  dashboardId: string
  version: number
  from: string
  to: string
  quickRanges: QuickRange[]
  hidden: boolean
  timezone: string | undefined
  defaultRange?: { spec: Dashboard; version: number } | null
}

export function TimeRangePicker({
  dashboardId,
  version,
  from,
  to,
  quickRanges,
  hidden,
  timezone,
  defaultRange,
}: Props) {
  const queryClient = useQueryClient()
  const setSearch = useSetDashboardSearch()
  const save = useAddVersion(dashboardId)

  function saveDefault(next: TimeRangeValue) {
    if (!defaultRange) return
    save.mutate(
      {
        spec: { ...defaultRange.spec, time: next },
        summary: `Set the default time range to ${describeRange(next.from, next.to, quickRanges, displayZone(undefined, timezone))}`,
        parent_version: defaultRange.version,
        propose: false,
      },
      {
        onSuccess: (outcome) => toast.success(saveToast(outcome)),
        onError: (error) =>
          toast.error(`Could not save the default. ${loadProblemText(error)}`),
      }
    )
  }

  return (
    <div className="flex items-center gap-1">
      {hidden ? null : (
        <TimeRangeControl
          value={{ from, to }}
          onChange={(next) => setSearch((state) => ({ ...state, ...next }))}
          quickRanges={quickRanges}
          zone={displayZone(undefined, timezone) ?? null}
          saveDefault={
            defaultRange
              ? {
                  saved: defaultRange.spec.time,
                  pending: save.isPending,
                  onSave: saveDefault,
                }
              : undefined
          }
        />
      )}
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
