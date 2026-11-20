import type { Folder } from "@/folders/queries"
import type { Dashboard } from "@/spec"

import type { DuplicateTargets } from "./duplicate"

const blankDashboardNote = [
  "This dashboard has no data panels yet.",
  "",
  "To add one, open **More** in the top bar and choose **Add panel**, then pick a datasource, write the query and save.",
  "",
  "Remove this note from its panel menu when you no longer need it.",
].join("\n")

export function blankDashboardSpec(title: string): Dashboard {
  return {
    specVersion: "0",
    title: title.trim(),
    time: { from: "now-1h", to: "now" },
    panels: [
      {
        id: "how-to-add-panels",
        type: "text",
        title: "Add your first panel",
        gridPos: { x: 0, y: 0, w: 12, h: 6 },
        content: blankDashboardNote,
      },
    ],
  }
}

export function blankDashboardTargets(
  folders: readonly Folder[]
): DuplicateTargets {
  const choices = folders.filter((folder) => folder.permissions.create)
  const initial =
    choices.find((folder) => folder.personal) ??
    choices.find((folder) => folder.general) ??
    choices[0]
  return { choices, initial: initial?.id ?? null }
}
