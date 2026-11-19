import { Link, useLocation } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"

export function NotFoundPage() {
  useDocumentTitle("Page not found - SlashObserve")
  const pathname = useLocation({ select: (location) => location.pathname })
  return (
    <>
      <PageHeader
        title="Page not found"
        description={
          <>
            There is nothing at <code className="font-mono">{pathname}</code>.
            Check the address, or start from Home or the dashboards.
          </>
        }
      />
      <div className="flex flex-wrap gap-2">
        <Link to="/" className={cn(buttonVariants({ variant: "outline" }))}>
          Back to home
        </Link>
        <Link
          to="/dashboards"
          className={cn(buttonVariants({ variant: "outline" }))}
        >
          Open dashboards
        </Link>
      </div>
    </>
  )
}
