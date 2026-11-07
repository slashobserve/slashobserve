import { Link } from "@tanstack/react-router"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import type { MouseEvent } from "react"

import { buttonVariants } from "@/components/ui/button"
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
} from "@/components/ui/pagination"
import { pageItems } from "@/lib/paging"
import { cn } from "@/lib/utils"

import { pageSearch, type ListSearch } from "./list-filters"

type ListPaginationProps = {
  page: number
  pageCount: number
  search: ListSearch
  onNavigate: (page: number) => void
}

const stepClass = cn(buttonVariants({ variant: "ghost" }), "gap-1 px-2")

function opensHere(event: MouseEvent): boolean {
  return !(event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
}

export function ListPagination({
  page,
  pageCount,
  search,
  onNavigate,
}: ListPaginationProps) {
  const go = (target: number) => (event: MouseEvent) => {
    if (opensHere(event)) onNavigate(target)
  }
  return (
    <Pagination aria-label="Dashboard pages" className="mx-0 w-auto">
      <PaginationContent>
        <PaginationItem>
          {page > 1 ? (
            <Link
              to="/dashboards"
              search={pageSearch(search, page - 1)}
              activeOptions={{ exact: true }}
              aria-label="Previous page"
              onClick={go(page - 1)}
              className={stepClass}
            >
              <ChevronLeftIcon aria-hidden="true" />
              <span className="narrow:sr-only">Previous</span>
            </Link>
          ) : (
            <span
              aria-disabled="true"
              className={cn(stepClass, "pointer-events-none opacity-50")}
            >
              <ChevronLeftIcon aria-hidden="true" />
              <span className="narrow:sr-only">Previous</span>
            </span>
          )}
        </PaginationItem>
        {pageItems(page, pageCount).map((item) => (
          <PaginationItem key={item}>
            {typeof item === "number" ? (
              <Link
                to="/dashboards"
                search={pageSearch(search, item)}
                activeOptions={{ exact: true }}
                aria-label={`Page ${item}`}
                onClick={item === page ? undefined : go(item)}
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
          {page < pageCount ? (
            <Link
              to="/dashboards"
              search={pageSearch(search, page + 1)}
              activeOptions={{ exact: true }}
              aria-label="Next page"
              onClick={go(page + 1)}
              className={stepClass}
            >
              <span className="narrow:sr-only">Next</span>
              <ChevronRightIcon aria-hidden="true" />
            </Link>
          ) : (
            <span
              aria-disabled="true"
              className={cn(stepClass, "pointer-events-none opacity-50")}
            >
              <span className="narrow:sr-only">Next</span>
              <ChevronRightIcon aria-hidden="true" />
            </span>
          )}
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  )
}
