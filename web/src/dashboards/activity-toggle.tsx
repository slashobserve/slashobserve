import { DockToggle } from "@/components/dock/dock-toggle"

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
  const label = `Activity${
    openCount > 0
      ? `, ${openCount} open ${openCount === 1 ? "comment" : "comments"}`
      : ""
  }`
  return (
    <DockToggle
      label={label}
      count={openCount}
      pressed={pressed}
      disabled={disabled}
      onToggle={onToggle}
    />
  )
}
