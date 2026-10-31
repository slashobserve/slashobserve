import {
  rowValue,
  type FieldConfig,
  type LinkScope,
  type ResolvedLink,
} from "../field-config"
import {
  actionRequest,
  parseActions,
  type ActionContext,
  type ActionValue,
  type PendingRequest,
  type RunnableAction,
} from "./actions"

export type FieldAction = RunnableAction & { override: number | null }

export type OneClickTarget =
  { kind: "link"; link: ResolvedLink } | { kind: "action"; action: FieldAction }

export function fieldActions(
  config: Pick<FieldConfig, "actions" | "actionsOverride">,
  scope: LinkScope
): FieldAction[] {
  return parseActions(config.actions, (name) => rowValue(scope, name)).map(
    (action) => ({ ...action, override: config.actionsOverride })
  )
}

export function fieldActionValue(
  scope: Pick<LinkScope, "value" | "valueText">
): ActionValue | null {
  const { value, valueText } = scope
  if (value === undefined && valueText === undefined) return null
  const raw = value === undefined || value === null ? "" : String(value)
  return {
    raw,
    text: valueText ?? raw,
    numeric: typeof value === "number" && Number.isFinite(value) ? value : null,
  }
}

export function fieldActionRequest({
  context,
  panelId,
  action,
  scope,
  fromMs,
  toMs,
}: {
  context: ActionContext
  panelId: string
  action: FieldAction
  scope: LinkScope
  fromMs: number
  toMs: number
}): PendingRequest {
  return actionRequest({
    context,
    panelId,
    target: { override: action.override },
    value: fieldActionValue(scope),
    action,
    vars: scope.vars,
    fromMs,
    toMs,
  })
}

export function oneClickTarget(
  links: ResolvedLink[],
  actions: FieldAction[]
): OneClickTarget | null {
  if (links.length + actions.length !== 1) return null
  if (links.length === 1) {
    return links[0].oneClick ? { kind: "link", link: links[0] } : null
  }
  return actions[0].oneClick ? { kind: "action", action: actions[0] } : null
}
