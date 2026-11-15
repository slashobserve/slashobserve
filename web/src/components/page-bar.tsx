import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
} from "@/components/ui/pagination"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  pageCountOf,
  pageItems,
  pageSizes,
  rangeText,
  type PageItem,
  type PageSize,
  type PageState,
} from "@/lib/paging"

const sizeOptions = pageSizes.map((size) => ({
  value: size,
  label: `${size} per page`,
}))

function numberedItems(state: PageState): PageItem[] {
  const count = pageCountOf(state)
  return count === null ? [state.page] : pageItems(state.page, count)
}

export function PageBar({
  state,
  noun,
  qualifier,
  note,
  onPage,
  onPageSize,
  busy,
  label,
}: {
  state: PageState
  noun: string
  qualifier?: string
  note?: string
  onPage: (page: number) => void
  onPageSize: (size: PageSize) => void
  busy?: boolean
  label: string
}) {
  const go = (page: number, enabled: boolean) => () => {
    if (enabled) onPage(page)
  }
  return (
    <div className="flex min-h-11 flex-wrap items-center justify-between gap-x-4 gap-y-2 py-1 text-xs">
      <div className="flex min-w-0 flex-col gap-0.5">
        <p>{rangeText(state, noun, qualifier)}</p>
        {note !== undefined ? (
          <p className="text-muted-foreground">{note}</p>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Select
          items={sizeOptions}
          value={state.pageSize}
          onValueChange={(size) => {
            if (size !== null) onPageSize(size)
          }}
        >
          <SelectTrigger
            size="sm"
            aria-label="Rows per page"
            className="narrow:data-[size=sm]:h-11"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="end">
            {sizeOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Pagination aria-label={label} aria-busy={busy} className="mx-0 w-auto">
          <PaginationContent>
            <PaginationItem>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label="Previous page"
                className="narrow:h-11"
                aria-disabled={!state.hasPrev}
                onClick={go(state.page - 1, state.hasPrev)}
              >
                <ChevronLeftIcon aria-hidden="true" />
                Previous
              </Button>
            </PaginationItem>
            {state.byOffset ? (
              [
                ...numberedItems(state).map((item) => (
                  <PaginationItem key={item}>
                    {typeof item === "number" ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-label={`Page ${item}`}
                        aria-current={item === state.page ? "page" : undefined}
                        onClick={
                          item === state.page ? undefined : go(item, true)
                        }
                        className="size-7 px-0 tabular-nums aria-[current=page]:bg-muted narrow:hidden"
                      >
                        {item}
                      </Button>
                    ) : (
                      <PaginationEllipsis className="narrow:hidden" />
                    )}
                  </PaginationItem>
                )),
                <PaginationItem
                  key="phone-page"
                  className="hidden narrow:block"
                >
                  <span className="px-2 tabular-nums">{`Page ${state.page}`}</span>
                </PaginationItem>,
              ]
            ) : (
              <PaginationItem>
                <span
                  aria-current="page"
                  className="px-2 tabular-nums"
                >{`Page ${state.page}`}</span>
              </PaginationItem>
            )}
            <PaginationItem>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label="Next page"
                className="narrow:h-11"
                aria-disabled={!state.hasNext}
                onClick={go(state.page + 1, state.hasNext)}
              >
                Next
                <ChevronRightIcon aria-hidden="true" />
              </Button>
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  )
}
