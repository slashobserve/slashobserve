import { Breadcrumb } from "./breadcrumb"
import { useSidebarStore } from "./sidebar-store"
import { ThemeSwitch } from "./theme-switch"

export function TopRow({
  setActionsHost,
}: {
  setActionsHost: (element: HTMLDivElement | null) => void
}) {
  const collapsed = useSidebarStore((state) => state.collapsed)
  const setCollapsed = useSidebarStore((state) => state.setCollapsed)
  return (
    <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 py-[9px] pr-3 pl-2.5">
      <div className="flex min-w-0 items-center gap-2">
        <button
          type="button"
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          aria-expanded={!collapsed}
          aria-controls="app-sidebar"
          onClick={() => setCollapsed(!collapsed)}
          className="grid size-7 place-items-center rounded-control text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring narrow:hidden"
        >
          <svg
            aria-hidden="true"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
          >
            <rect x="3" y="4" width="18" height="16" rx="2" />
            <path d="M9 4v16" />
          </svg>
        </button>
        <Breadcrumb />
      </div>
      <div className="flex items-center gap-1.5">
        <div ref={setActionsHost} className="flex items-center gap-1.5" />
        <ThemeSwitch />
      </div>
    </header>
  )
}
