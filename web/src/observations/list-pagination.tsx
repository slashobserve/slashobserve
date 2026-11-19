import type { ReactNode } from "react"
import { Link } from "@tanstack/react-router"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"

import { buttonVariants } from "@/components/ui/button"
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
} from "@/components/ui/pagination"
import { pageItems } from "@/lib/paging"
import { cn } from "@/lib/utils"

import { searchOf, type ObservationListView } from "./list-view"

type Props = {
  view: ObservationListView
  pageCount: number
}

const stepClass = cn(buttonVariants({ variant: "ghost" }), "gap-1 px-2")

function Step({
  view,
  page,
  label,
  children,
}: {
  view: ObservationListView
  page: number | null
  label: string
  children: ReactNode
}) {
  if (page === null) {
    return (
      <span
        aria-disabled="true"
        className={cn(stepClass, "pointer-events-none opacity-50")}
      >
        {children}
      </span>
    )
  }
  return (
    <Link
      to="/observations"
      search={searchOf({ ...view, page }, false)}
      aria-label={label}
      className={stepClass}
    >
      {children}
    </Link>
  )
}

export function ObservationsPagination({ view, pageCount }: Props) {
  const { page } = view
  return (
    <Pagination aria-label="Observation pages" className="mx-0 w-auto">
      <PaginationContent>
        <PaginationItem>
          <Step
            view={view}
            page={page > 1 ? page - 1 : null}
            label="Previous page"
          >
            <ChevronLeftIcon aria-hidden="true" />
            <span className="narrow:sr-only">Previous</span>
          </Step>
        </PaginationItem>
        {pageItems(page, pageCount).map((item) => (
          <PaginationItem key={item}>
            {typeof item === "number" ? (
              <Link
                to="/observations"
                search={searchOf({ ...view, page: item }, false)}
                aria-label={`Page ${item}`}
                aria-current={item === page ? "page" : undefined}
                className={cn(
                  buttonVariants({
                    variant: item === page ? "outline" : "ghost",
                  }),
                  "min-w-8 px-2 tabular-nums"
                )}
              >
                {item}
              </Link>
            ) : (
              <PaginationEllipsis />
            )}
          </PaginationItem>
        ))}
        <PaginationItem>
          <Step
            view={view}
            page={page < pageCount ? page + 1 : null}
            label="Next page"
          >
            <span className="narrow:sr-only">Next</span>
            <ChevronRightIcon aria-hidden="true" />
          </Step>
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  )
}
