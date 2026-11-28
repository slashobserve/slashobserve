import { oneClickProps } from "@/dashboards/dashboard-links"

import {
  isDataLink,
  resolveLink,
  type FieldConfig,
  type LinkScope,
  type ResolvedLink,
} from "../field-config"
import { runFieldAction, type StableActionRunner } from "./action-runner"
import { fieldActions, oneClickTarget, type FieldAction } from "./field-actions"

export type ItemInteraction = {
  links: ResolvedLink[]
  actions: FieldAction[]
  oneClick: ReturnType<typeof oneClickProps>
  run: (action: FieldAction) => void
}

export function itemInteraction(
  item: {
    config: Pick<FieldConfig, "links" | "actions" | "actionsOverride">
  },
  scope: LinkScope,
  runner: StableActionRunner,
  clickable = true
): ItemInteraction {
  const links = item.config.links
    .filter(isDataLink)
    .flatMap((link) => resolveLink(link, scope) ?? [])
  const actions = fieldActions(item.config, scope)
  const run = (action: FieldAction) => runFieldAction(runner, action, scope)
  const target = clickable ? oneClickTarget(links, actions) : null
  const oneClick =
    target === null
      ? {}
      : target.kind === "link"
        ? oneClickProps(links)
        : runner.reason === null
          ? oneClickProps([], () => run(target.action))
          : {}
  return { links, actions, oneClick, run }
}
