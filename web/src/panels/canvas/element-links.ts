import type { Link } from "@/spec"

import { resolveLink, type LinkScope, type ResolvedLink } from "../field-config"
import type { ActionValue, ElementAction } from "./actions"

export type OneClick = "link" | "action"

export type Target =
  | { kind: "link"; link: ResolvedLink }
  | { kind: "action"; action: ElementAction }

export type Interaction =
  | { mode: "none" }
  | { mode: "direct"; target: Target }
  | { mode: "menu"; targets: Target[] }

export type Interactive = {
  kind: string
  oneClick: OneClick | null
  actions: ElementAction[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

export function parseLinks(raw: unknown): Link[] {
  if (!Array.isArray(raw)) return []
  return raw.flatMap((entry): Link[] => {
    if (!isRecord(entry)) return []
    const { title, url, newTab } = entry
    if (typeof title !== "string" || typeof url !== "string") return []
    if (title === "" || url === "") return []
    return [{ title, url, ...(typeof newTab === "boolean" ? { newTab } : {}) }]
  })
}

export function resolveElementLinks(
  links: Link[],
  value: ActionValue | null,
  scope: LinkScope
): ResolvedLink[] {
  const bound: LinkScope =
    value === null
      ? scope
      : { ...scope, value: value.raw, valueText: value.text }
  return links.flatMap((link) => resolveLink(link, bound) ?? [])
}

function targetsOf(element: Interactive, links: ResolvedLink[]): Target[] {
  const actions = element.actions.map((action): Target => ({
    kind: "action",
    action,
  }))
  if (element.kind === "button" && actions.length > 0) return actions
  return [...links.map((link): Target => ({ kind: "link", link })), ...actions]
}

export function interactionOf(
  element: Interactive,
  links: ResolvedLink[]
): Interaction {
  const targets = targetsOf(element, links)
  if (targets.length === 0) return { mode: "none" }
  const preferred =
    element.kind === "button"
      ? undefined
      : targets.find((target) => target.kind === element.oneClick)
  if (preferred) return { mode: "direct", target: preferred }
  if (targets.length === 1) return { mode: "direct", target: targets[0] }
  return { mode: "menu", targets }
}
