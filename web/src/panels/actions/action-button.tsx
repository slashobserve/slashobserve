import { useId, useMemo, type ComponentProps } from "react"

import { Button } from "@/components/ui/button"
import { useTheme } from "@/lib/theme"

import { resolveColor } from "../colors"
import { cssColorToRgb, readableText } from "../contrast"
import { useActionContext } from "./action-context"
import { disabledReason, type RunnableAction } from "./actions"

export function ActionButton({
  action,
  onRun,
  size = "sm",
}: {
  action: RunnableAction
  onRun: () => void
  size?: ComponentProps<typeof Button>["size"]
}) {
  const reason = disabledReason(useActionContext())
  const { resolved } = useTheme()
  const reasonId = useId()
  const text = useMemo(
    () => {
      if (action.color === null) return undefined
      const rgb = cssColorToRgb(resolveColor(action.color))
      return rgb ? readableText(rgb) : undefined
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resolveColor reads the live theme tokens, so a theme flip must recompute the text color
    [action.color, resolved]
  )
  const blocked = reason !== null
  return (
    <span title={reason ?? undefined} className="inline-flex min-w-0">
      <Button
        size={size}
        variant={action.color === null ? "outline" : "default"}
        disabled={blocked}
        focusableWhenDisabled
        aria-describedby={blocked ? reasonId : undefined}
        className="min-w-0"
        style={
          action.color === null
            ? undefined
            : { backgroundColor: action.color, color: text }
        }
        onClick={onRun}
      >
        <span className="truncate">{action.title}</span>
      </Button>
      {blocked ? (
        <span id={reasonId} className="sr-only">
          {reason}
        </span>
      ) : null}
    </span>
  )
}
