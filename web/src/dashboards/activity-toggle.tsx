import { DockToggle } from "@/components/dock/dock-toggle"

import { useCloseControls } from "./controls-sheet"
import { activityButtonId } from "./pane-openers"

type Props = {
  pressed: boolean
  openCount: number
  disabled?: boolean
  onToggle: () => void
}

export function ActivityToggle({
  pressed,
  openCount,
  disabled,
  onToggle,
}: Props) {
  const closeControls = useCloseControls()
  const label = `Activity${
    openCount > 0
      ? `, ${openCount} open ${openCount === 1 ? "comment" : "comments"}`
      : ""
  }`
  return (
    <DockToggle
      id={activityButtonId}
      label={label}
      count={openCount}
      pressed={pressed}
      disabled={disabled ?? false}
      onToggle={() => {
        closeControls()
        onToggle()
      }}
    />
  )
}
