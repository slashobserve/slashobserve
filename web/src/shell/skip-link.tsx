import { focusPage } from "./focus-after-navigation"

export function SkipLink() {
  return (
    <a
      href="#main"
      onClick={(event) => {
        event.preventDefault()
        focusPage(document)
      }}
      className="fixed top-2 left-2 z-50 -translate-y-16 rounded-control border bg-background px-3 py-2 text-sm text-foreground no-underline outline-none focus:translate-y-0 focus-visible:ring-2 focus-visible:ring-ring"
    >
      Skip to the page
    </a>
  )
}
