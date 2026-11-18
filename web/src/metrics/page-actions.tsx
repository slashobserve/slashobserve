import { CopyButton } from "@/components/copy-button"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { TopRowActions } from "@/shell/top-row-actions"

type Props = {
  range: { from: string; to: string }
  onRangeChange: (next: { from: string; to: string }) => void
}

export function MetricsPageActions({ range, onRangeChange }: Props) {
  return (
    <TopRowActions>
      <div className="flex items-center gap-2">
        <span className="contents narrow:[&_button]:h-11">
          <TimeRangeControl value={range} onChange={onRangeChange} />
        </span>
        <CopyButton
          text={window.location.href}
          label="Copy link"
          variant="ghost"
          size="icon"
          className="narrow:size-11"
        />
      </div>
    </TopRowActions>
  )
}
