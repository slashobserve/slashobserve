import { useId, type Dispatch } from "react"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { isDataPanel } from "@/spec"
import { cn } from "@/lib/utils"

import { DisplayOptions } from "./display-options"
import type { DraftAction, EditablePanel } from "./draft"
import { ThresholdsEditor } from "./thresholds-editor"
import { TypeSelect } from "./type-select"
import { UnitSelect } from "./unit-select"

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
  const title = (
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
  )
  if (draft.type === "dashlist") {
    return (
      <div className="flex flex-col gap-3.5">
        {title}
        <DisplayOptions
          type="dashlist"
          display={draft.display}
          dispatch={dispatch}
          readOnly={readOnly}
        />
      </div>
    )
  }
  if (!isDataPanel(draft)) {
    return <div className="flex flex-col gap-3.5">{title}</div>
  }
  return (
    <div className="flex flex-col gap-3.5">
      <TypeSelect type={draft.type} dispatch={dispatch} readOnly={readOnly} />
      <UnitSelect unit={draft.unit} dispatch={dispatch} readOnly={readOnly} />
      <ThresholdsEditor
        thresholds={draft.thresholds}
        dispatch={dispatch}
        readOnly={readOnly}
      />
      {title}
      <DisplayOptions
        type={draft.type}
        display={draft.display}
        dispatch={dispatch}
        readOnly={readOnly}
      />
    </div>
  )
}
