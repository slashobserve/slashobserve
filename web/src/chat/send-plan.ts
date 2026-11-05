import type { ObserveInput } from "@/observe/queries"

import type { SlashCommand } from "./slash-commands"

export type Opens = "build" | "chat" | "same"

export type SendPlan = { input: ObserveInput; opens: Opens }

export function planSend({
  command,
  text,
  dashboardId,
}: {
  command: SlashCommand | null
  text: string
  dashboardId?: string
}): SendPlan | { error: string } {
  switch (command) {
    case "investigate":
      return { error: "Investigate is not available yet." }
    case "import":
      return { input: { prompt: text }, opens: "build" }
    case "revise":
      if (dashboardId === undefined) {
        return { error: "Open a dashboard to revise it." }
      }
      return { input: { prompt: text, dashboardId }, opens: "same" }
    case null:
      return dashboardId === undefined
        ? { input: { prompt: text }, opens: "build" }
        : { input: { prompt: text, dashboardId }, opens: "same" }
  }
}
