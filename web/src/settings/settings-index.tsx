import { useEffect, useState } from "react"
import { Link } from "@tanstack/react-router"

import { cn } from "@/lib/utils"

export type SettingsSection = { id: string; label: string }

export function SettingsIndex({
  sections,
}: {
  sections: readonly SettingsSection[]
}) {
  const current = useSectionInView(sections)
  return (
    <nav
      aria-label="On this page"
      className="sticky top-6 hidden w-44 flex-none flex-col gap-0.5 xl:flex"
    >
      {sections.map((section) => (
        <Link
          key={section.id}
          to="/settings"
          search={true}
          hash={section.id}
          aria-current={section.id === current ? "location" : undefined}
          className={cn(
            "flex min-h-7 items-center rounded-control px-2.5 text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
            section.id === current && "text-foreground"
          )}
        >
          {section.label}
        </Link>
      ))}
    </nav>
  )
}

const upperPartOfView = "0px 0px -60% 0px"

function useSectionInView(sections: readonly SettingsSection[]) {
  const [current, setCurrent] = useState(sections[0]?.id)
  useEffect(() => {
    const visible = new Set<string>()
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target.id)
          else visible.delete(entry.target.id)
        }
        const first = sections.find((section) => visible.has(section.id))
        if (first !== undefined) setCurrent(first.id)
      },
      { rootMargin: upperPartOfView }
    )
    for (const section of sections) {
      const element = document.getElementById(section.id)
      if (element !== null) observer.observe(element)
    }
    return () => observer.disconnect()
  }, [sections])
  return current
}
