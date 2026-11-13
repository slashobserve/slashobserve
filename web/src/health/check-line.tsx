import { RelativeTime } from "@/components/relative-time"

import type { HealthItem } from "./health-labels"

export function CheckLine({ item }: { item: HealthItem }) {
  if (item.skipped === "depends_on_viewer")
    return <>Skipped: reads the viewer</>
  if (item.checked_at === null) return <>Not checked yet</>
  return (
    <>
      Checked <RelativeTime iso={item.checked_at} />
      {item.checked_version === null
        ? null
        : ` on version ${item.checked_version}`}
      {item.state === "outdated" ? ", before the latest version" : null}
    </>
  )
}
