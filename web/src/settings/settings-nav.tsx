import { Fragment } from "react"
import { Link, useNavigate } from "@tanstack/react-router"

import { RoleNote } from "@/components/role-note"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

import type { SettingsGroup, SettingsSectionId } from "./sections"

const closePanels = (prev: { connection?: string; sync?: string }) => ({
  ...prev,
  panel: undefined,
  webhook: undefined,
})

export function SettingsNav({
  groups,
  selected,
  member,
}: {
  groups: readonly SettingsGroup[]
  selected: SettingsSectionId
  member: boolean
}) {
  return (
    <nav
      aria-label="Settings sections"
      className="sticky top-0 flex w-[200px] flex-none flex-col gap-0.5 narrow:hidden"
    >
      {groups.map((group, index) => (
        <Fragment key={group.label}>
          <div
            className={cn(
              "px-2.5 pb-1 text-xs text-muted-foreground",
              index > 0 && "pt-3"
            )}
          >
            {group.label}
          </div>
          {group.sections.map((section) => (
            <Link
              key={section.id}
              to="/settings"
              search={closePanels}
              hash={section.id}
              activeOptions={{ includeHash: true }}
              aria-current={section.id === selected ? "page" : undefined}
              className="flex h-7 items-center rounded-control px-2.5 text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-muted aria-[current=page]:font-medium aria-[current=page]:text-foreground"
            >
              {section.label}
            </Link>
          ))}
        </Fragment>
      ))}
      {member ? (
        <RoleNote className="mx-2.5 mt-3.5 items-start text-xs">
          Admins also manage Grafana, webhooks and agent access here.
        </RoleNote>
      ) : null}
    </nav>
  )
}

export function SettingsSectionSelect({
  groups,
  selected,
}: {
  groups: readonly SettingsGroup[]
  selected: SettingsSectionId
}) {
  const navigate = useNavigate()
  const items = groups.flatMap((group) =>
    group.sections.map((section) => ({
      value: section.id,
      label: section.label,
    }))
  )
  const selectedGroup = groups.find((group) =>
    group.sections.some((section) => section.id === selected)
  )
  return (
    <Select
      items={items}
      value={selected}
      onValueChange={(next: SettingsSectionId | null) => {
        if (next === null) return
        void navigate({ to: "/settings", hash: next, search: closePanels })
      }}
    >
      <SelectTrigger
        aria-label="Settings section"
        className="hidden h-11 w-full narrow:flex"
      >
        <span className="text-muted-foreground">{selectedGroup?.label}</span>
        <SelectValue className="flex-1 text-left font-medium" />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {groups.map((group) => (
          <SelectGroup key={group.label}>
            <SelectLabel>{group.label}</SelectLabel>
            {group.sections.map((section) => (
              <SelectItem
                key={section.id}
                value={section.id}
                className="min-h-11"
              >
                {section.label}
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  )
}
