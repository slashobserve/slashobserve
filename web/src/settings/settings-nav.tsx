import { Link } from "@tanstack/react-router"

import { cn } from "@/lib/utils"

import type { SettingsSection, SettingsSectionId } from "./sections"

const placements = {
  side: {
    nav: "sticky top-6 hidden w-44 flex-none flex-col gap-0.5 xl:flex",
    link: "",
  },
  top: {
    nav: "mb-4 flex gap-1 overflow-x-auto xl:hidden",
    link: "flex-none whitespace-nowrap focus-visible:ring-inset",
  },
}

export function SettingsNav({
  sections,
  selected,
  placement,
}: {
  sections: readonly SettingsSection[]
  selected: SettingsSectionId
  placement: keyof typeof placements
}) {
  return (
    <nav aria-label="Settings sections" className={placements[placement].nav}>
      {sections.map((section) => (
        <Link
          key={section.id}
          to="/settings"
          search={true}
          hash={section.id}
          activeOptions={{ includeHash: true }}
          aria-current={section.id === selected ? "page" : undefined}
          className={cn(
            "flex min-h-7 items-center rounded-control px-2.5 text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-muted aria-[current=page]:text-foreground",
            placements[placement].link
          )}
        >
          {section.label}
        </Link>
      ))}
    </nav>
  )
}
