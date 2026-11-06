import type { ObserveInput } from "@/observe/queries"

import type { SlashCommand } from "./slash-commands"

export type Opens = "build" | "chat" | "same"

export type SendPlan = { input: ObserveInput; opens: Opens }

type SendContext = {
  command: SlashCommand | null
  text: string
  dashboardId?: string
  panelId?: string
  from?: string
  to?: string
}

function investigation({
  text,
  dashboardId,
  panelId,
  from,
  to,
}: SendContext): SendPlan {
  const input: ObserveInput = { prompt: text, kind: "investigate" }
  if (dashboardId !== undefined) input.dashboardId = dashboardId
  if (dashboardId !== undefined && panelId !== undefined) {
    input.panelId = panelId
  }
  if (from !== undefined && to !== undefined) {
    input.from = from
    input.to = to
  }
  return { input, opens: dashboardId === undefined ? "chat" : "same" }
}

export function planSend(context: SendContext): SendPlan | { error: string } {
  const { command, text, dashboardId } = context
  switch (command) {
    case "investigate":
      return investigation(context)
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
