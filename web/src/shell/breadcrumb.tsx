import { Link } from "@tanstack/react-router"

import { useCrumbs } from "./crumbs"

export function Breadcrumb({
  setTitleHost,
}: {
  setTitleHost: (element: HTMLSpanElement | null) => void
}) {
  const { crumbs, pageTitle } = useCrumbs()
  const last = crumbs.length - 1
  return (
    <nav
      aria-label="Breadcrumb"
      className="flex min-w-0 items-center gap-2 text-[13px]"
    >
      {crumbs.map((crumb, index) =>
        index === last && !pageTitle ? (
          <span
            key={crumb.label}
            aria-current="page"
            className="min-w-0 truncate font-medium"
          >
            {crumb.label}
          </span>
        ) : (
          <span key={crumb.label} className="contents">
            {crumb.to ? (
              <Link
                to={crumb.to}
                className="rounded-control text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              >
                {crumb.label}
              </Link>
            ) : (
              <span className="text-muted-foreground">{crumb.label}</span>
            )}
            <span aria-hidden="true" className="text-muted-foreground">
              /
            </span>
          </span>
        )
      )}
      {pageTitle ? <span ref={setTitleHost} className="contents" /> : null}
    </nav>
  )
}
