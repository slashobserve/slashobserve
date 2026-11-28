import { useState } from "react"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  dayLabel,
  dayText,
  monthTitle,
  monthWeeks,
  selectedSpan,
  shiftMonth,
  weekdayLetters,
  type DaySelection,
} from "@/lib/calendar"
import { cn } from "@/lib/utils"

type Props = {
  initial: { year: number; month: number }
  selection: DaySelection
  onPick: (day: string) => void
}

const weekdayNames = [
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
  "Sunday",
]

export function DateCalendar({ initial, selection, onPick }: Props) {
  const [shown, setShown] = useState(initial)
  const span = selectedSpan(selection)
  const title = monthTitle(shown.year, shown.month)

  return (
    <div role="group" aria-label="Calendar" className="flex flex-col gap-1">
      <div className="flex items-center justify-between">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Previous month"
          onClick={() => setShown(shiftMonth(shown.year, shown.month, -1))}
        >
          <ChevronLeftIcon />
        </Button>
        <span aria-live="polite" className="text-[13px] font-medium">
          {title}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Next month"
          onClick={() => setShown(shiftMonth(shown.year, shown.month, 1))}
        >
          <ChevronRightIcon />
        </Button>
      </div>
      <table
        aria-label={title}
        className="w-full border-separate border-spacing-0.5"
      >
        <thead>
          <tr>
            {weekdayLetters.map((letter, index) => (
              <th
                key={weekdayNames[index]}
                scope="col"
                abbr={weekdayNames[index]}
                className="size-7 text-xs font-normal text-muted-foreground"
              >
                {letter}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {monthWeeks(shown.year, shown.month).map((week) => (
            <tr key={week[0] === undefined ? undefined : dayText(week[0])}>
              {week.map((day) => {
                const text = dayText(day)
                const endpoint =
                  span !== null && (text === span.first || text === span.last)
                const between =
                  span !== null && text > span.first && text < span.last
                return (
                  <td key={text} className="p-0 text-center">
                    <button
                      type="button"
                      aria-label={dayLabel(day)}
                      aria-pressed={endpoint || between}
                      onClick={() => onPick(text)}
                      className={cn(
                        "size-7 rounded-control text-xs tabular-nums outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
                        day.month !== shown.month && "text-muted-foreground",
                        between && "bg-selection",
                        endpoint &&
                          "bg-primary text-primary-foreground hover:bg-primary-hover"
                      )}
                    >
                      {day.day}
                    </button>
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
