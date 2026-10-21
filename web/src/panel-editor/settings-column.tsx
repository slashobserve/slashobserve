import { useId, type Dispatch } from "react"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

import type { DraftAction, EditablePanel } from "./draft"

export type SettingsColumnProps = {
  draft: EditablePanel
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
}

const maxTitleLength = 200

export function SettingsColumn({
  draft,
  dispatch,
  readOnly,
}: SettingsColumnProps) {
  const titleId = useId()
  return (
    <div className="flex flex-col gap-3.5">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={titleId} className="text-[13px]">
          Title
        </Label>
        <Input
          id={titleId}
          value={draft.title ?? ""}
          maxLength={maxTitleLength}
          readOnly={readOnly}
          className={cn(readOnly && "text-muted-foreground")}
          onChange={(event) =>
            dispatch({ type: "setTitle", title: event.target.value })
          }
        />
      </div>
    </div>
  )
}
